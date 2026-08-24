package command

import (
	"bytes"
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/exec"
	"github.com/ousiassllc/gsr-helper/internal/exec/mask"
)

// 監査ログに秘匿値が出ないこと（docs/architecture/security.md「秘匿値の扱い」）を
// 検証する。
//
// レコードの中身そのもの（audit_test.go）と分けているのは、**壊れたときの影響が
// 違う**ためである。あちらが守るのは「記録が残ること」で、こちらが守るのは
// 「記録に残してはいけないものが残らないこと」である。1 ファイル 300 行の上限
// （docs/ui/atomic-design.md）に収める際の切れ目もここになる（Issue #111）。

func TestCommandRunMasksSecretInAuditError(t *testing.T) {
	const secret = "supersecrettoken"

	var buf bytes.Buffer
	c := New(func() []string { return []string{secret} }, WithAudit(testLogger(&buf)))

	// 引数にも標準エラー出力にも secret を出す。監査レコードの error はコマンド行
	// だけを載せるため、args のマスクが効いていること・stderr が載らないことの
	// 両方をここで固定する。
	name, args := helperCommand("--token", secret)
	ctx := exec.WithOptions(context.Background(), exec.Options{
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
	if !strings.Contains(rec.Error, mask.Placeholder) {
		t.Errorf("監査ログの error がマスクされていない: %s", rec.Error)
	}
	if strings.Contains(rec.Error, "token is") {
		t.Errorf("監査ログの error に標準エラー出力が載っている: %s", rec.Error)
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
	ctx := exec.WithOptions(context.Background(), exec.Options{
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
