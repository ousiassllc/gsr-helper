package command

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/audit"
	"github.com/ousiassllc/gsr-helper/internal/exec"
)

// captureAuditErrorSink は通知先未設定時の書き出し先を差し替える。
// 実際の os.Stderr へ書かせるとテストから内容を確かめられないためである。
func captureAuditErrorSink(t *testing.T) *bytes.Buffer {
	t.Helper()

	prev := auditErrorSink
	var buf bytes.Buffer
	auditErrorSink = &buf
	t.Cleanup(func() { auditErrorSink = prev })
	return &buf
}

// runWithFailingAudit は必ず書き込みに失敗する監査ログで、成功するコマンドを実行する。
func runWithFailingAudit(t *testing.T, opts ...Option) (exec.Result, error) {
	t.Helper()

	c := New(NoSecrets, append([]Option{WithAudit(audit.New(failWriter{}))}, opts...)...)
	name, args := helperCommand()
	ctx := exec.WithOptions(context.Background(), exec.Options{Env: helperEnv(helperStdoutEnv + "=ok")})
	return c.Run(ctx, name, args...)
}

func TestCommandRunAuditFailureNotified(t *testing.T) {
	var notified []error
	res, err := runWithFailingAudit(t, WithAuditErrorFunc(func(e error) { notified = append(notified, e) }))

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

	var aerr *AuditError
	if !errors.As(notified[0], &aerr) {
		t.Fatalf("*AuditError が通知されていない: %v", notified[0])
	}
	if !strings.Contains(aerr.Error(), "監査ログ") {
		t.Errorf("通知内容が監査ログの失敗と分からない: %v", aerr)
	}
	if aerr.Unwrap() == nil {
		t.Error("基の書き込みエラーに到達できない")
	}
}

func TestCommandRunAuditFailureFallsBackToStderr(t *testing.T) {
	sink := captureAuditErrorSink(t)
	res, err := runWithFailingAudit(t)

	// 通知先が未設定でも、成功したコマンドを失敗として返してはならない。
	if err != nil {
		t.Fatalf("記録の失敗が Run のエラーに混ざっている: %v", err)
	}
	if got := string(res.Stdout); got != "ok" {
		t.Errorf("Stdout = %q, want %q", got, "ok")
	}
	// それでも黙って消さない。最後の砦として標準エラー出力へ 1 行出す。
	if lines := strings.Count(sink.String(), "\n"); lines != 1 {
		t.Errorf("標準エラー出力への行数 = %d, want 1: %q", lines, sink.String())
	}
	if !strings.Contains(sink.String(), "監査ログ") {
		t.Errorf("記録の失敗と分かる出力になっていない: %q", sink.String())
	}
}

func TestCommandRunAuditFailureKeepsCommandError(t *testing.T) {
	// コマンド自体が失敗した場合は、その失敗だけがそのまま返る（記録の失敗は混ざらない）。
	sink := captureAuditErrorSink(t)

	c := New(NoSecrets, WithAudit(audit.New(failWriter{})))
	name, args := helperCommand()
	ctx := exec.WithOptions(context.Background(), exec.Options{Env: helperEnv(helperExitEnv + "=3")})

	_, err := c.Run(ctx, name, args...)

	var exitErr *ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("*ExitError が返っていない: %v", err)
	}
	var aerr *AuditError
	if errors.As(err, &aerr) {
		t.Error("記録の失敗が返り値に混ざっている")
	}
	if !strings.Contains(sink.String(), "監査ログ") {
		t.Errorf("記録の失敗が通知されていない: %q", sink.String())
	}
}
