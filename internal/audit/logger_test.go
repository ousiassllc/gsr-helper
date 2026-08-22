package audit

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// countingWriter は Write の呼び出し回数を数える Writer。
// 1 レコードが 1 回の Write で書かれることを検証するために使う。
type countingWriter struct {
	buf   bytes.Buffer
	calls int
}

func (w *countingWriter) Write(p []byte) (int, error) {
	w.calls++
	return w.buf.Write(p)
}

// errWriter は必ず失敗する Writer。
type errWriter struct{}

func (errWriter) Write([]byte) (int, error) {
	return 0, errors.New("書き込み不可")
}

func TestLoggerWriteSpecExamples(t *testing.T) {
	tests := []struct {
		name string
		at   time.Time
		rec  Record
		want string
	}{
		{
			name: "svc.stop",
			at:   time.Date(2026, 8, 21, 12, 0, 0, 0, jst()),
			rec: Record{
				Action:     "svc.stop",
				Runner:     "build01-2",
				Dir:        "/opt/runners/build01-2",
				Command:    []string{"systemctl", "stop", "actions.runner.foo-bar.build01-2.service"},
				DurationMS: 412,
			},
			want: `{"ts":"2026-08-21T12:00:00+09:00","uid":0,"sudo_user":"ousiass","action":"svc.stop","runner":"build01-2","dir":"/opt/runners/build01-2","command":["systemctl","stop","actions.runner.foo-bar.build01-2.service"],"exit_code":0,"duration_ms":412}`,
		},
		{
			name: "runner.add",
			at:   time.Date(2026, 8, 21, 12, 1, 20, 0, jst()),
			rec: Record{
				Action: "runner.add",
				Runner: "build01-4",
				Dir:    "/opt/runners/build01-4",
				Command: []string{
					"./config.sh", "--url", "https://github.com/orgs/foo", "--token", "***",
					"--name", "build01-4", "--labels", "self-hosted,linux,x64", "--unattended",
				},
				DurationMS: 3180,
			},
			want: `{"ts":"2026-08-21T12:01:20+09:00","uid":0,"sudo_user":"ousiass","action":"runner.add","runner":"build01-4","dir":"/opt/runners/build01-4","command":["./config.sh","--url","https://github.com/orgs/foo","--token","***","--name","build01-4","--labels","self-hosted,linux,x64","--unattended"],"exit_code":0,"duration_ms":3180}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			lg := New(&buf, WithClock(fixedClock(tt.at)), WithIdentity(0, "ousiass"))
			if err := lg.Write(tt.rec); err != nil {
				t.Fatalf("Write がエラーを返した: %v", err)
			}
			if got := buf.String(); got != tt.want+"\n" {
				t.Errorf("仕様の実例と一致しない\n got: %s\nwant: %s", got, tt.want)
			}
		})
	}
}

func TestLoggerWriteEmptyFields(t *testing.T) {
	var buf bytes.Buffer
	lg := New(&buf, WithClock(fixedClock(time.Date(2026, 8, 21, 12, 0, 0, 0, jst()))), WithIdentity(1000, ""))
	if err := lg.Write(Record{Action: "doctor.check", Command: []string{"gh", "auth", "status"}}); err != nil {
		t.Fatalf("Write がエラーを返した: %v", err)
	}

	got := buf.String()
	// runner / dir は空でもキーを残す（後段の集計でキーの有無を場合分けさせない）。
	for _, want := range []string{`"sudo_user":""`, `"runner":""`, `"dir":""`, `"uid":1000`} {
		if !strings.Contains(got, want) {
			t.Errorf("%s を含まない: %s", want, got)
		}
	}
	// error は空なら出さない。
	if strings.Contains(got, `"error"`) {
		t.Errorf("error キーが出力されている: %s", got)
	}
}

func TestLoggerWriteIsSingleCall(t *testing.T) {
	w := &countingWriter{}
	lg := New(w)
	for range 3 {
		if err := lg.Write(Record{Action: "svc.start"}); err != nil {
			t.Fatalf("Write がエラーを返した: %v", err)
		}
	}

	if w.calls != 3 {
		t.Errorf("Write の呼び出し回数 = %d, want 3（1 レコード 1 回）", w.calls)
	}
	if !strings.HasSuffix(w.buf.String(), "\n") {
		t.Error("末尾が改行で終わっていない")
	}
	if got := strings.Count(w.buf.String(), "\n"); got != 3 {
		t.Errorf("行数 = %d, want 3", got)
	}
}

func TestLoggerWriteConcurrent(t *testing.T) {
	const n = 100

	var buf bytes.Buffer
	lg := New(&buf)

	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := lg.Write(Record{Action: "svc.stop", ExitCode: i}); err != nil {
				t.Errorf("Write がエラーを返した: %v", err)
			}
		}()
	}
	wg.Wait()

	lines := strings.Split(strings.TrimSuffix(buf.String(), "\n"), "\n")
	if len(lines) != n {
		t.Fatalf("行数 = %d, want %d", len(lines), n)
	}
	for i, line := range lines {
		var rec map[string]any
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("%d 行目が JSON として読めない: %v (%s)", i+1, err, line)
		}
	}
}

func TestLoggerWriteTimestampOrderMatchesLineOrder(t *testing.T) {
	// 時刻の取得がロックの外にあると、同時に Write した goroutine 同士で ts の
	// 順序と行の順序が入れ替わり、監査ログを時系列として読めなくなる。
	const n = 200

	var calls atomic.Int64
	base := time.Date(2026, 8, 21, 12, 0, 0, 0, jst())
	clock := func() time.Time {
		return base.Add(time.Duration(calls.Add(1)) * time.Second)
	}

	var buf bytes.Buffer
	lg := New(&buf, WithClock(clock))

	var wg sync.WaitGroup
	for range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := lg.Write(Record{Action: "svc.stop"}); err != nil {
				t.Errorf("Write がエラーを返した: %v", err)
			}
		}()
	}
	wg.Wait()

	lines := strings.Split(strings.TrimSuffix(buf.String(), "\n"), "\n")
	if len(lines) != n {
		t.Fatalf("行数 = %d, want %d", len(lines), n)
	}
	var prev time.Time
	for i, line := range lines {
		var rec struct {
			TS Timestamp `json:"ts"`
		}
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("%d 行目が JSON として読めない: %v (%s)", i+1, err, line)
		}
		ts := time.Time(rec.TS)
		if i > 0 && !ts.After(prev) {
			t.Fatalf("%d 行目の ts = %s が前の行 %s より後になっていない", i+1, ts, prev)
		}
		prev = ts
	}
}

func TestLoggerNoOp(t *testing.T) {
	tests := map[string]*Logger{
		"Discard": Discard(),
		"nil":     nil,
	}
	for name, lg := range tests {
		t.Run(name, func(t *testing.T) {
			if err := lg.Write(Record{Action: "svc.stop"}); err != nil {
				t.Errorf("Write がエラーを返した: %v", err)
			}
			if err := lg.Close(); err != nil {
				t.Errorf("Close がエラーを返した: %v", err)
			}
		})
	}
}

func TestLoggerWriteError(t *testing.T) {
	lg := New(errWriter{})
	if err := lg.Write(Record{Action: "svc.stop"}); err == nil {
		t.Fatal("書き込み失敗時にエラーを返していない")
	}
}

func TestLoggerCloseDoesNotCloseGivenWriter(t *testing.T) {
	var buf bytes.Buffer
	lg := New(&buf)
	if err := lg.Close(); err != nil {
		t.Fatalf("Close がエラーを返した: %v", err)
	}
	// New に渡した Writer は閉じないため、Close 後も書き込みは続けられる。
	if err := lg.Write(Record{Action: "svc.stop"}); err != nil {
		t.Fatalf("Close 後の Write がエラーを返した: %v", err)
	}
	if buf.Len() == 0 {
		t.Error("Close 後の Write が書き込まれていない")
	}
}

func TestWithClockIgnoresNil(t *testing.T) {
	// nil を採ると Write で落ちるため既定の time.Now を保つ。
	var buf bytes.Buffer
	lg := New(&buf, WithClock(nil))
	if err := lg.Write(Record{Action: "svc.stop"}); err != nil {
		t.Fatalf("Write がエラーを返した: %v", err)
	}
	if !strings.Contains(buf.String(), `"ts":"`) {
		t.Errorf("ts が書かれていない: %s", buf.String())
	}
}
