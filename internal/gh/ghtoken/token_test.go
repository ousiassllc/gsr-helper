package ghtoken_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ousiassllc/gsr-helper/internal/exec"
	"github.com/ousiassllc/gsr-helper/internal/gh/ghtoken"
)

// env は差し替え用の環境変数マップから Getenv を作る。
func env(kv map[string]string) func(string) string {
	return func(k string) string { return kv[k] }
}

// found は「PATH 上にある」を返す LookPath の差し替え。
func found(name string) (string, error) { return "/usr/bin/" + name, nil }

// missing は「PATH 上に無い」を返す LookPath の差し替え。
func missing(string) (string, error) { return "", errors.New("見つかりません") }

// okResult は終了コード 0 と標準出力を返す結果を作る。
func okResult(out string) exec.Result {
	return exec.Result{Stdout: []byte(out), Stderr: nil, ExitCode: 0}
}

func TestTokenPrefersEnvAndIssuesNoCommand(t *testing.T) {
	t.Parallel()

	f := exec.NewFake()
	src := ghtoken.Source{
		Exec:     f,
		Getenv:   env(map[string]string{ghtoken.EnvToken: "  env-token-value  "}),
		Geteuid:  func() int { return 0 },
		LookPath: found,
		Timeout:  0,
	}

	got, err := src.Token(context.Background())
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if got != "env-token-value" {
		t.Errorf("トークン = %q, want %q（前後の空白を落とすこと）", got, "env-token-value")
	}
	if calls := f.Calls(); len(calls) != 0 {
		t.Errorf("環境変数で分かるのにコマンドを発行している: %v", calls)
	}
}

func TestTokenUsesSudoPathWhenRootWithSudoUser(t *testing.T) {
	t.Parallel()

	f := exec.NewFake()
	f.SetFunc(func(name string, _ []string) (exec.Result, error) {
		if name == "sudo" {
			return okResult("sudo-token\n"), nil
		}
		return exec.Result{Stdout: nil, Stderr: nil, ExitCode: 1}, nil
	})

	src := ghtoken.Source{
		Exec:     f,
		Getenv:   env(map[string]string{"SUDO_USER": "ousiass"}),
		Geteuid:  func() int { return 0 },
		LookPath: found,
		Timeout:  0,
	}

	got, err := src.Token(context.Background())
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if got != "sudo-token" {
		t.Errorf("トークン = %q, want %q", got, "sudo-token")
	}

	calls := f.Calls()
	if len(calls) != 1 {
		t.Fatalf("発行コマンド数 = %d, want 1: %v", len(calls), calls)
	}
	if want := "sudo -u ousiass gh auth token"; calls[0].String() != want {
		t.Errorf("発行コマンド = %q, want %q", calls[0].String(), want)
	}
	if calls[0].Options.SkipAudit {
		t.Error("トークン取得に SkipAudit が付いている（記録対象である）")
	}
}

func TestTokenFallsBackToDirectGh(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		envs  map[string]string
		euid  int
		wants []string
	}{
		"SUDO_USER が無い": {
			envs:  map[string]string{},
			euid:  0,
			wants: []string{"gh auth token"},
		},
		"root ではない": {
			envs:  map[string]string{"SUDO_USER": "ousiass"},
			euid:  1000,
			wants: []string{"gh auth token"},
		},
		"SUDO_USER の文字種が不正": {
			envs:  map[string]string{"SUDO_USER": "bad user!"},
			euid:  0,
			wants: []string{"gh auth token"},
		},
		"sudo 経路が失敗したら直接実行に落ちる": {
			envs:  map[string]string{"SUDO_USER": "ousiass"},
			euid:  0,
			wants: []string{"sudo -u ousiass gh auth token", "gh auth token"},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			f := exec.NewFake()
			f.SetFunc(func(cmd string, _ []string) (exec.Result, error) {
				if cmd == "sudo" {
					return exec.Result{Stdout: nil, Stderr: nil, ExitCode: 1}, nil
				}
				return okResult("direct-token\n"), nil
			})

			src := ghtoken.Source{
				Exec:     f,
				Getenv:   env(tt.envs),
				Geteuid:  func() int { return tt.euid },
				LookPath: found,
				Timeout:  0,
			}

			got, err := src.Token(context.Background())
			if err != nil {
				t.Fatalf("err = %v, want nil", err)
			}
			if got != "direct-token" {
				t.Errorf("トークン = %q, want %q", got, "direct-token")
			}

			calls := f.Calls()
			if len(calls) != len(tt.wants) {
				t.Fatalf("発行コマンド数 = %d, want %d: %v", len(calls), len(tt.wants), calls)
			}
			for i, want := range tt.wants {
				if calls[i].String() != want {
					t.Errorf("発行コマンド[%d] = %q, want %q", i, calls[i].String(), want)
				}
			}
		})
	}
}

func TestTokenReturnsErrNoTokenWhenNothingWorks(t *testing.T) {
	t.Parallel()

	f := exec.NewFake()
	f.SetFunc(func(string, []string) (exec.Result, error) {
		return exec.Result{Stdout: nil, Stderr: nil, ExitCode: 1}, nil
	})

	src := ghtoken.Source{Exec: f, Getenv: env(nil), Geteuid: func() int { return 1000 }, LookPath: found, Timeout: 0}

	_, err := src.Token(context.Background())
	if !errors.Is(err, ghtoken.ErrNoToken) {
		t.Errorf("err = %v, want ErrNoToken", err)
	}
}

func TestTokenTreatsEmptyStdoutAsFailure(t *testing.T) {
	t.Parallel()

	f := exec.NewFake()
	f.SetFunc(func(string, []string) (exec.Result, error) { return okResult("   \n"), nil })

	src := ghtoken.Source{Exec: f, Getenv: env(nil), Geteuid: func() int { return 1000 }, LookPath: found, Timeout: 0}

	if _, err := src.Token(context.Background()); !errors.Is(err, ghtoken.ErrNoToken) {
		t.Errorf("終了コード 0 でも標準出力が空なら失敗として扱うこと: err = %v", err)
	}
}

func TestTokenSkipsCommandsWhenGhIsMissing(t *testing.T) {
	t.Parallel()

	f := exec.NewFake()
	src := ghtoken.Source{
		Exec:     f,
		Getenv:   env(nil),
		Geteuid:  func() int { return 0 },
		LookPath: missing,
		Timeout:  0,
	}

	if _, err := src.Token(context.Background()); !errors.Is(err, ghtoken.ErrNoToken) {
		t.Errorf("err = %v, want ErrNoToken", err)
	}
	if calls := f.Calls(); len(calls) != 0 {
		t.Errorf("gh が無いのにコマンドを発行している（無駄な監査ログが残る）: %v", calls)
	}
}

func TestTokenWithoutExecutorOnlyReadsEnv(t *testing.T) {
	t.Parallel()

	src := ghtoken.Source{Exec: nil, Getenv: env(nil), Geteuid: func() int { return 0 }, Timeout: 0}
	if _, err := src.Token(context.Background()); !errors.Is(err, ghtoken.ErrNoToken) {
		t.Errorf("Executor が無ければ ErrNoToken を返すこと: err = %v", err)
	}
}

func TestTokenAppliesPerCommandTimeout(t *testing.T) {
	t.Parallel()

	f := exec.NewFake()
	f.SetFunc(func(string, []string) (exec.Result, error) { return okResult("tok"), nil })

	src := ghtoken.Source{
		Exec:     f,
		Getenv:   env(nil),
		Geteuid:  func() int { return 1000 },
		LookPath: found,
		Timeout:  50 * time.Millisecond,
	}

	if _, err := src.Token(context.Background()); err != nil {
		t.Fatalf("err = %v", err)
	}

	calls := f.Calls()
	if len(calls) != 1 {
		t.Fatalf("発行コマンド数 = %d, want 1", len(calls))
	}
	if !calls[0].HasDeadline {
		t.Error("1 コマンドあたりの期限が設定されていない")
	}
}

func TestHasTokenDoesNotReturnValue(t *testing.T) {
	t.Parallel()

	f := exec.NewFake()
	f.SetFunc(func(string, []string) (exec.Result, error) { return okResult("secret-token"), nil })

	if !ghtoken.HasToken(context.Background(), f, 100*time.Millisecond) {
		t.Error("HasToken = false, want true")
	}

	f2 := exec.NewFake()
	f2.SetFunc(func(string, []string) (exec.Result, error) {
		return exec.Result{Stdout: nil, Stderr: nil, ExitCode: 1}, nil
	})
	if ghtoken.HasToken(context.Background(), f2, 100*time.Millisecond) {
		t.Error("HasToken = true, want false")
	}
}
