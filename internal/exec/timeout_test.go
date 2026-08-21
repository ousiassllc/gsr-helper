package exec

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestCommandRunTimesOut(t *testing.T) {
	name, args := helperCommand()
	ctx := WithOptions(context.Background(), Options{
		Action: "test.timeout",
		Env:    helperEnv(helperSleepEnv + "=10000"),
	})

	start := time.Now()
	res, err := New(NoSecrets, WithTimeout(150*time.Millisecond)).Run(ctx, name, args...)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("タイムアウトしたのにエラーを返していない")
	}
	if !strings.Contains(err.Error(), "タイムアウト") {
		t.Errorf("タイムアウトと分かるエラーになっていない: %v", err)
	}
	if res.ExitCode != -1 {
		t.Errorf("ExitCode = %d, want -1", res.ExitCode)
	}
	if elapsed > 5*time.Second {
		t.Errorf("中断までに %v かかった（タイムアウトが効いていない）", elapsed)
	}
}

func TestCommandRunTakesMinOfCallerDeadlineAndTimeout(t *testing.T) {
	// 1 コマンドあたりの上限（WithTimeout）は保険なので、呼び出し側の deadline で
	// 上書きせず min を採る。操作全体に長い deadline を張った呼び出し側が、個々の
	// コマンドの上限まで緩めてしまわないようにするためである。
	tests := []struct {
		name           string
		callerTimeout  time.Duration
		commandTimeout time.Duration
	}{
		{
			name:           "既定より短い呼び出し側 deadline を尊重する",
			callerTimeout:  150 * time.Millisecond,
			commandTimeout: 30 * time.Second,
		},
		{
			name:           "既定より長い呼び出し側 deadline は既定まで短縮する",
			callerTimeout:  30 * time.Second,
			commandTimeout: 150 * time.Millisecond,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name, args := helperCommand()
			ctx := WithOptions(context.Background(), Options{Env: helperEnv(helperSleepEnv + "=10000")})
			ctx, cancel := context.WithTimeout(ctx, tt.callerTimeout)
			defer cancel()

			start := time.Now()
			res, err := New(NoSecrets, WithTimeout(tt.commandTimeout)).Run(ctx, name, args...)
			elapsed := time.Since(start)

			if err == nil {
				t.Fatal("タイムアウトしたのにエラーを返していない")
			}
			if !strings.Contains(err.Error(), "タイムアウト") {
				t.Errorf("タイムアウトと分かるエラーになっていない: %v", err)
			}
			if res.ExitCode != -1 {
				t.Errorf("ExitCode = %d, want -1", res.ExitCode)
			}
			if elapsed > 5*time.Second {
				t.Errorf("中断までに %v かかった（min が採られていない）", elapsed)
			}
		})
	}
}

func TestCommandRunCompletesWithinCallerDeadline(t *testing.T) {
	name, args := helperCommand()
	ctx := WithOptions(context.Background(), Options{Env: helperEnv(helperSleepEnv+"=100", helperStdoutEnv+"=done")})

	// deadline と既定のどちらにも収まる実行はそのまま完走する。
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	res, err := New(NoSecrets).Run(ctx, name, args...)
	if err != nil {
		t.Fatalf("Run がエラーを返した: %v", err)
	}
	if got := string(res.Stdout); got != "done" {
		t.Errorf("Stdout = %q, want %q", got, "done")
	}
}

func TestCommandRunCanceledWhileRunning(t *testing.T) {
	name, args := helperCommand()
	ctx := WithOptions(context.Background(), Options{Env: helperEnv(helperSleepEnv + "=10000")})
	ctx, cancel := context.WithCancel(ctx)

	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()
	defer cancel()

	start := time.Now()
	res, err := New(NoSecrets).Run(ctx, name, args...)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("キャンセルしたのにエラーを返していない")
	}
	if !strings.Contains(err.Error(), "キャンセル") {
		t.Errorf("キャンセルと分かるエラーになっていない: %v", err)
	}
	if res.ExitCode != -1 {
		t.Errorf("ExitCode = %d, want -1", res.ExitCode)
	}
	if elapsed > 5*time.Second {
		t.Errorf("中断までに %v かかった（キャンセルが効いていない）", elapsed)
	}
}

func TestCommandRunCanceledBeforeStart(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	name, args := helperCommand()
	res, err := New(NoSecrets).Run(WithOptions(ctx, Options{Env: helperEnv()}), name, args...)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("context.Canceled を包んだエラーになっていない: %v", err)
	}
	if res.ExitCode != -1 {
		t.Errorf("ExitCode = %d, want -1", res.ExitCode)
	}
}
