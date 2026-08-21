package exec

import (
	"bytes"
	"context"
	"slices"
	"testing"
	"time"
)

func TestWithTimeout(t *testing.T) {
	// 0 や負値をそのまま採ると即タイムアウトする実行になるため既定を保つ。
	for _, d := range []time.Duration{0, -1, -time.Hour} {
		if got := New(NoSecrets, WithTimeout(d)).timeout; got != defaultTimeout {
			t.Errorf("WithTimeout(%v) 後の timeout = %v, want %v", d, got, defaultTimeout)
		}
	}
	if got := New(NoSecrets, WithTimeout(time.Second)).timeout; got != time.Second {
		t.Errorf("timeout = %v, want 1s", got)
	}
}

func TestWithAuditIgnoresNil(t *testing.T) {
	// nil を採ると記録先が消えるため既定の Discard を保つ。
	c := New(NoSecrets, WithAudit(nil))
	if c.auditLog == nil {
		t.Fatal("WithAudit(nil) で記録先が nil になっている")
	}

	name, args := helperCommand()
	ctx := WithOptions(context.Background(), Options{Env: helperEnv(helperStdoutEnv + "=ok")})
	if _, err := c.Run(ctx, name, args...); err != nil {
		t.Fatalf("Run がエラーを返した: %v", err)
	}
}

func TestWithAuditOverridesDiscard(t *testing.T) {
	var buf bytes.Buffer
	c := New(NoSecrets, WithAudit(nil), WithAudit(testLogger(&buf)))

	name, args := helperCommand()
	ctx := WithOptions(context.Background(), Options{Env: helperEnv()})
	if _, err := c.Run(ctx, name, args...); err != nil {
		t.Fatalf("Run がエラーを返した: %v", err)
	}
	if buf.Len() == 0 {
		t.Error("後から渡した記録先に書かれていない")
	}
}

func TestNewNilSecretsBehavesAsNoSecrets(t *testing.T) {
	// nil は panic させず NoSecrets と同等に扱う（起動時に落とさない方を選ぶ）。
	if got := NoSecrets(); got != nil {
		t.Errorf("NoSecrets() = %q, want nil", got)
	}
	if got := New(nil).secrets(); got != nil {
		t.Errorf("New(nil).secrets() = %q, want nil", got)
	}

	// 値一致マスクは効かないが、キー名ベースのマスクは New だけで必ず効く。
	want := []string{"--token", maskPlaceholder}
	if got := New(nil).MaskArgs([]string{"--token", "supersecrettoken"}); !slices.Equal(got, want) {
		t.Errorf("MaskArgs() = %q, want %q", got, want)
	}
}
