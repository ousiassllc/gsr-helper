package exec

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ousiassllc/gsr-helper/internal/audit"
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
	ctx := WithOptions(context.Background(), Options{
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

	want := append([]string{name}, "-test.run=^TestHelperProcess$", "--", "--token", maskPlaceholder, "--name", "build01-4")
	if !slices.Equal(rec.Command, want) {
		t.Errorf("command = %q, want %q", rec.Command, want)
	}
}

func TestCommandRunAuditRecordOnStartFailure(t *testing.T) {
	var buf bytes.Buffer
	c := New(NoSecrets, WithAudit(testLogger(&buf)))

	ctx := WithOptions(context.Background(), Options{Action: "svc.stop", Runner: "build01-2"})
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

func TestCommandRunMasksSecretInAuditError(t *testing.T) {
	const secret = "supersecrettoken"

	var buf bytes.Buffer
	c := New(func() []string { return []string{secret} }, WithAudit(testLogger(&buf)))

	name, args := helperCommand()
	ctx := WithOptions(context.Background(), Options{
		Action: "runner.add",
		Env:    helperEnv(helperExitEnv+"=1", helperStderrEnv+"=token is "+secret),
	})

	res, err := c.Run(ctx, name, args...)
	if err == nil {
		t.Fatal("非ゼロ終了でエラーを返していない")
	}
	// Stdout / Stderr はマスクしない。UI が実出力を見る必要があるため。
	if !strings.Contains(string(res.Stderr), secret) {
		t.Errorf("Stderr がマスクされている: %q", res.Stderr)
	}

	rec := decodeAudit(t, &buf)
	if strings.Contains(rec.Error, secret) {
		t.Errorf("監査ログの error にトークンが残っている: %s", rec.Error)
	}
	if !strings.Contains(rec.Error, maskPlaceholder) {
		t.Errorf("監査ログの error がマスクされていない: %s", rec.Error)
	}
}

func TestCommandRunAuditFailureNotified(t *testing.T) {
	var notified []error
	c := New(
		NoSecrets,
		WithAudit(audit.New(failWriter{})),
		WithAuditErrorFunc(func(err error) { notified = append(notified, err) }),
	)

	name, args := helperCommand()
	ctx := WithOptions(context.Background(), Options{Env: helperEnv(helperStdoutEnv + "=ok")})

	res, err := c.Run(ctx, name, args...)
	// 記録の失敗は通知先へ流し、Run の返り値はコマンド自体の成否だけを表す。
	if err != nil {
		t.Fatalf("Run がエラーを返した: %v", err)
	}
	if got := string(res.Stdout); got != "ok" {
		t.Errorf("Stdout = %q, want %q", got, "ok")
	}
	if len(notified) != 1 {
		t.Fatalf("通知件数 = %d, want 1", len(notified))
	}
	if !strings.Contains(notified[0].Error(), "監査ログ") {
		t.Errorf("通知内容が監査ログの失敗と分からない: %v", notified[0])
	}
}

func TestCommandRunAuditFailureJoinedWithoutHandler(t *testing.T) {
	c := New(NoSecrets, WithAudit(audit.New(failWriter{})))

	name, args := helperCommand()
	ctx := WithOptions(context.Background(), Options{Env: helperEnv(helperStdoutEnv + "=ok")})

	res, err := c.Run(ctx, name, args...)
	// 通知先が未設定のときは黙って消さず Run のエラーに合成する。
	if err == nil {
		t.Fatal("通知先が無いのにエラーが消えている")
	}
	if got := string(res.Stdout); got != "ok" {
		t.Errorf("Stdout = %q, want %q", got, "ok")
	}
}

func TestCommandRunWithoutAuditOption(t *testing.T) {
	// 既定は audit.Discard() のため、監査ログを設定しなくても実行できる。
	name, args := helperCommand()
	ctx := WithOptions(context.Background(), Options{Env: helperEnv(helperStdoutEnv + "=ok")})

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
	ctx := WithOptions(context.Background(), Options{
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

func TestCommandRunCallsSecretsProviderOnce(t *testing.T) {
	// provider は並行安全を求められるためロック取得が入り得るし、Run の途中で
	// 返り値が変わると ExitError.Args と監査ログの command でマスク結果が
	// 食い違う。1 回の Run につき 1 度だけ取ることを固定する。
	const secret = "supersecrettoken"

	var mu sync.Mutex
	calls := 0
	provider := func() []string {
		mu.Lock()
		defer mu.Unlock()
		calls++
		return []string{secret}
	}

	var buf bytes.Buffer
	c := New(provider, WithAudit(testLogger(&buf)))

	name, args := helperCommand("--token", secret)
	ctx := WithOptions(context.Background(), Options{
		Action: "runner.add",
		Env:    helperEnv(helperExitEnv+"=1", helperStderrEnv+"=token is "+secret),
	})

	if _, err := c.Run(ctx, name, args...); err == nil {
		t.Fatal("非ゼロ終了でエラーを返していない")
	}
	if calls != 1 {
		t.Errorf("provider の呼び出し回数 = %d, want 1", calls)
	}
	if rec := decodeAudit(t, &buf); strings.Contains(rec.Error, secret) {
		t.Errorf("マスク漏れがある: %s", rec.Error)
	}
}
