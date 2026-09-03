package audit

import (
	"bytes"
	"strings"
	"testing"
)

// Report は Write と違って戻り値を持たない（Issue #71）。ここではその縮退と
// 通知先の振る舞いだけを検証する。行の内容やキー順は logger_test.go の
// TestLoggerWriteSpecExamples 等が既に検証しているため重複させない。

func TestLoggerReportWritesLine(t *testing.T) {
	var buf bytes.Buffer
	lg := New(&buf)

	lg.Report(Record{Action: "disk.clean", Command: []string{"(削除)", "/opt/runners/build01-1/_work/repo"}})

	lines := strings.Split(strings.TrimSuffix(buf.String(), "\n"), "\n")
	if len(lines) != 1 {
		t.Fatalf("行数 = %d, want 1（%q）", len(lines), buf.String())
	}
	if !strings.Contains(lines[0], `"action":"disk.clean"`) {
		t.Errorf("action が書かれていない: %s", lines[0])
	}
}

// TestLoggerReportNotifiesErrorFunc は書き込み失敗時に WithErrorFunc の通知先へ
// 渡り、Report 自体は戻り値を持たない（呼び出し側からは失敗が見えない）ことを
// 確かめる。
func TestLoggerReportNotifiesErrorFunc(t *testing.T) {
	var notified []error
	lg := New(errWriter{}, WithErrorFunc(func(err error) { notified = append(notified, err) }))

	lg.Report(Record{Action: "disk.clean"})

	if len(notified) != 1 {
		t.Fatalf("通知回数 = %d, want 1", len(notified))
	}
	if notified[0] == nil {
		t.Error("通知されたエラーが nil")
	}
}

// TestLoggerReportFallsBackToStderr は WithErrorFunc 未設定時に os.Stderr 相当の
// 代替先へ 1 行出すことを確かめる（黙って握りつぶさない）。
func TestLoggerReportFallsBackToStderr(t *testing.T) {
	var sink bytes.Buffer
	orig := reportErrorSink
	reportErrorSink = &sink
	defer func() { reportErrorSink = orig }()

	lg := New(errWriter{})
	lg.Report(Record{Action: "disk.clean"})

	if sink.Len() == 0 {
		t.Error("代替先に何も書かれていない")
	}
}

// TestLoggerReportNoOp は nil / Discard() で Report が panic せず no-op になる
// ことを確かめる（Write と同じ縮退）。
func TestLoggerReportNoOp(t *testing.T) {
	tests := map[string]*Logger{
		"Discard": Discard(),
		"nil":     nil,
	}
	for name, lg := range tests {
		t.Run(name, func(*testing.T) {
			// panic せずに戻ってくることだけを確かめる（戻り値が無いため）。
			lg.Report(Record{Action: "disk.clean"})
		})
	}
}
