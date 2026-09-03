package command

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ousiassllc/gsr-helper/internal/audit"
	"github.com/ousiassllc/gsr-helper/internal/exec"
	"github.com/ousiassllc/gsr-helper/internal/exec/mask"
)

// auditLine は書き出された監査レコードを読み戻すための型。
type auditLine struct {
	TS         string   `json:"ts"`
	UID        int      `json:"uid"`
	SudoUser   string   `json:"sudo_user"`
	Action     string   `json:"action"`
	Runner     string   `json:"runner"`
	Dir        string   `json:"dir"`
	Command    []string `json:"command"`
	ExitCode   int      `json:"exit_code"`
	DurationMS int64    `json:"duration_ms"`
	Error      string   `json:"error"`
}

// failWriter は必ず失敗する Writer。監査記録の失敗を再現する。
type failWriter struct{}

func (failWriter) Write([]byte) (int, error) {
	return 0, errors.New("書き込み不可")
}

// testLogger は固定の時刻と識別子で書き出す Logger を返す。
func testLogger(buf *bytes.Buffer) *audit.Logger {
	at := time.Date(2026, 8, 21, 12, 0, 0, 0, time.FixedZone("JST", 9*60*60))
	return audit.New(buf, audit.WithClock(func() time.Time { return at }), audit.WithIdentity(0, "ousiass"))
}

// decodeAudit は 1 行だけ書かれていることを確かめて読み戻す。
func decodeAudit(t *testing.T, buf *bytes.Buffer) auditLine {
	t.Helper()
	lines := strings.Split(strings.TrimSuffix(buf.String(), "\n"), "\n")
	if len(lines) != 1 {
		t.Fatalf("監査ログの行数 = %d, want 1: %q", len(lines), buf.String())
	}

	var rec auditLine
	if err := json.Unmarshal([]byte(lines[0]), &rec); err != nil {
		t.Fatalf("監査レコードが読めない: %v (%s)", err, lines[0])
	}
	return rec
}

func TestCommandRunWritesAuditRecord(t *testing.T) {
	const secret = "supersecrettoken"

	dir := t.TempDir()
	var buf bytes.Buffer
	c := New(func() []string { return []string{secret} }, WithAudit(testLogger(&buf)))

	name, args := helperCommand("--token", secret, "--name", "build01-4")
	ctx := exec.WithOptions(context.Background(), exec.Options{
		Action: "runner.add",
		Runner: "build01-4",
		Dir:    dir,
		Env:    helperEnv(),
	})

	if _, err := c.Run(ctx, name, args...); err != nil {
		t.Fatalf("Run がエラーを返した: %v", err)
	}

	rec := decodeAudit(t, &buf)
	if rec.TS != "2026-08-21T12:00:00+09:00" || rec.UID != 0 || rec.SudoUser != "ousiass" {
		t.Errorf("ts / uid / sudo_user が Logger の値と一致しない: %+v", rec)
	}
	if rec.Action != "runner.add" || rec.Runner != "build01-4" || rec.Dir != dir {
		t.Errorf("ctx の Options と一致しない: %+v", rec)
	}
	if rec.ExitCode != 0 || rec.Error != "" {
		t.Errorf("成功時のレコードが想定と異なる: %+v", rec)
	}

	want := append([]string{name}, "-test.run=^TestHelperProcess$", "--", "--token", mask.Placeholder, "--name", "build01-4")
	if !slices.Equal(rec.Command, want) {
		t.Errorf("command = %q, want %q", rec.Command, want)
	}
}

func TestCommandRunAuditRecordOnStartFailure(t *testing.T) {
	var buf bytes.Buffer
	c := New(NoSecrets, WithAudit(testLogger(&buf)))

	ctx := exec.WithOptions(context.Background(), exec.Options{Action: "svc.stop", Runner: "build01-2"})
	if _, err := c.Run(ctx, "gsr-helper-no-such-command"); err == nil {
		t.Fatal("起動できないコマンドでエラーを返していない")
	}

	rec := decodeAudit(t, &buf)
	if rec.ExitCode != -1 {
		t.Errorf("exit_code = %d, want -1", rec.ExitCode)
	}
	if rec.Error == "" {
		t.Error("error が記録されていない")
	}
	if rec.Action != "svc.stop" || rec.Runner != "build01-2" || rec.Dir != "" {
		t.Errorf("Options が反映されていない: %+v", rec)
	}
}

func TestCommandRunWithoutAuditOption(t *testing.T) {
	// 既定は audit.Discard() のため、監査ログを設定しなくても実行できる。
	name, args := helperCommand()
	ctx := exec.WithOptions(context.Background(), exec.Options{Env: helperEnv(helperStdoutEnv + "=ok")})

	if _, err := New(NoSecrets).Run(ctx, name, args...); err != nil {
		t.Fatalf("Run がエラーを返した: %v", err)
	}
}

func TestCommandRunDoesNotRecordStdout(t *testing.T) {
	// gh auth token の stdout はトークンそのものなので、監査ログに stdout を
	// 載せないことが本パッケージで最も重要な不変条件。値一致マスクを持たない
	// NoSecrets で実行し、マスクに頼らず構造として安全であることを固定する。
	const secret = "supersecrettoken"

	var buf bytes.Buffer
	c := New(NoSecrets, WithAudit(testLogger(&buf)))

	name, args := helperCommand()
	ctx := exec.WithOptions(context.Background(), exec.Options{
		Action: "gh.token",
		Env:    helperEnv(helperStdoutEnv + "=" + secret),
	})

	res, err := c.Run(ctx, name, args...)
	if err != nil {
		t.Fatalf("Run がエラーを返した: %v", err)
	}
	if got := string(res.Stdout); got != secret {
		t.Fatalf("ヘルパーが secret を stdout に出していない: %q", got)
	}

	if strings.Contains(buf.String(), secret) {
		t.Errorf("監査ログに stdout が載っている: %s", buf.String())
	}
	if rec := decodeAudit(t, &buf); rec.Action != "gh.token" {
		t.Errorf("action = %q, want gh.token", rec.Action)
	}
}

func TestCommandRunWritesAuditRecordWithoutOptions(t *testing.T) {
	// Options が ctx に無くても実行と記録は成立する（action が空のレコードとして残る）。
	// メタ情報の欠落で記録漏れにしないという options.go の設計意図を固定する。
	var buf bytes.Buffer
	c := New(NoSecrets, WithAudit(testLogger(&buf)))

	name, args := helperCommand()
	if _, err := c.Run(context.Background(), name, args...); err != nil {
		t.Fatalf("Run がエラーを返した: %v", err)
	}

	rec := decodeAudit(t, &buf)
	if rec.Action != "" || rec.Runner != "" || rec.Dir != "" {
		t.Errorf("Options 未設定なのにメタ情報が入っている: %+v", rec)
	}
	if rec.ExitCode != 0 || len(rec.Command) == 0 {
		t.Errorf("実行結果が記録されていない: %+v", rec)
	}
}

// SkipAudit を指定した実行は監査ログに 1 行も出さない。再検出（Scan）が繰り返し
// 発行する読み取りコマンドが破壊的操作のレコードを押し流さないための指定である。
// 自動更新か手動再読み込み（r）かは問わない。起動に失敗した場合も出さない
// （失敗も同じだけ積み上がるため）。
func TestCommandRunSkipAudit(t *testing.T) {
	name, args := helperCommand()

	tests := []struct {
		name string
		run  func(*Command) error
	}{
		{"成功する実行", func(c *Command) error {
			ctx := exec.WithOptions(context.Background(), exec.Options{
				Action: "runner.discover", Env: helperEnv(), SkipAudit: true,
			})
			_, err := c.Run(ctx, name, args...)
			return err
		}},
		{"起動できない実行", func(c *Command) error {
			ctx := exec.WithOptions(context.Background(), exec.Options{
				Action: "runner.discover", SkipAudit: true,
			})
			_, err := c.Run(ctx, "gsr-helper-no-such-command")
			if err == nil {
				return errors.New("起動できないコマンドでエラーを返していない")
			}
			return nil
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			c := New(NoSecrets, WithAudit(testLogger(&buf)))
			if err := tt.run(c); err != nil {
				t.Fatal(err)
			}
			if buf.Len() != 0 {
				t.Errorf("監査ログに書かれた: %q", buf.String())
			}
		})
	}

	// 既定は記録する側。SkipAudit を指定しない同じ実行は 1 行残る
	// （記録漏れが既定にならないことを固定する）。
	var buf bytes.Buffer
	c := New(NoSecrets, WithAudit(testLogger(&buf)))
	ctx := exec.WithOptions(context.Background(), exec.Options{
		Action: "runner.discover", Env: helperEnv(),
	})
	if _, err := c.Run(ctx, name, args...); err != nil {
		t.Fatalf("Run がエラーを返した: %v", err)
	}
	if rec := decodeAudit(t, &buf); rec.Action != "runner.discover" {
		t.Errorf("action = %q, want runner.discover", rec.Action)
	}
}
