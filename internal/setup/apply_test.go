package setup_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/exec"
	"github.com/ousiassllc/gsr-helper/internal/setup"
	"github.com/ousiassllc/gsr-helper/internal/setup/setuptest"
)

func TestApplyIssuesPlannedCommandsWithRealToken(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	f := exec.NewFake()

	res, err := setup.Apply(context.Background(), setup.ApplyInput{
		Exec:     f,
		Plan:     setuptest.AddPlanIn(t, base, 1),
		Token:    "AREGISTRATIONTOKEN",
		TokenFor: nil,
		Tarball:  setuptest.MakeTarball(t),
		Drain:    nil,
		Progress: nil,
	})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if !res.OK() {
		t.Fatalf("Result = %+v, want 成功", res)
	}

	dir := filepath.Join(base, "build01-5")
	want := []string{
		"./config.sh --url https://github.com/orgs/foo --token AREGISTRATIONTOKEN " +
			"--name build01-5 --labels gpu --work _work --runnergroup Default --unattended",
		"./svc.sh install",
		"./svc.sh start",
	}
	if got := setuptest.Issued(f); !slices.Equal(got, want) {
		t.Errorf("発行コマンド:\n got: %v\nwant: %v", got, want)
	}

	for _, c := range f.Calls() {
		if c.Options.Action != setup.ActionAdd {
			t.Errorf("action = %q, want %q", c.Options.Action, setup.ActionAdd)
		}
		if c.Options.Runner != "build01-5" {
			t.Errorf("runner = %q, want build01-5", c.Options.Runner)
		}
		if c.Options.Dir != dir {
			t.Errorf("dir = %q, want %q", c.Options.Dir, dir)
		}
		if c.Options.SkipAudit {
			t.Error("破壊的操作に SkipAudit が付いている")
		}
	}
}

func TestApplyCreatesDirectoryAndExtractsTarball(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	if _, err := setup.Apply(context.Background(), setup.ApplyInput{
		Exec:     exec.NewFake(),
		Plan:     setuptest.AddPlanIn(t, base, 1),
		Token:    "TOKENTOKENTOKEN",
		TokenFor: nil,
		Tarball:  setuptest.MakeTarball(t),
		Drain:    nil,
		Progress: nil,
	}); err != nil {
		t.Fatalf("err = %v", err)
	}

	for _, name := range []string{"config.sh", "svc.sh", "bin/runnerversion"} {
		if _, err := os.Stat(filepath.Join(base, "build01-5", name)); err != nil {
			t.Errorf("展開されていない: %s (%v)", name, err)
		}
	}
}

func TestApplyStopsAtFailureAndKeepsSucceeded(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	f := exec.NewFake()
	// 2 台目の config.sh を失敗させる。1 台目は 3 本成功する。
	n := 0
	f.SetFunc(func(name string, _ []string) (exec.Result, error) {
		if name == "./config.sh" {
			n++
			if n == 2 {
				return exec.Result{Stdout: nil, Stderr: []byte("Forbidden"), ExitCode: 1},
					errors.New("config.sh が終了コード 1 で失敗しました")
			}
		}
		return exec.Result{Stdout: nil, Stderr: nil, ExitCode: 0}, nil
	})

	res, err := setup.Apply(context.Background(), setup.ApplyInput{
		Exec:     f,
		Plan:     setuptest.AddPlanIn(t, base, 3),
		Token:    "TOKENTOKENTOKEN",
		TokenFor: nil,
		Tarball:  setuptest.MakeTarball(t),
		Drain:    nil,
		Progress: nil,
	})
	if err == nil {
		t.Fatal("err = nil, want エラー")
	}

	if want := []string{"build01-5"}; !slices.Equal(res.Succeeded, want) {
		t.Errorf("成功 = %v, want %v", res.Succeeded, want)
	}
	if res.Failed != "build01-6" {
		t.Errorf("失敗した台 = %q, want build01-6", res.Failed)
	}
	if res.Phase != "登録" {
		t.Errorf("失敗フェーズ = %q, want 登録", res.Phase)
	}
	if want := []string{"build01-7"}; !slices.Equal(res.Remaining, want) {
		t.Errorf("未実行 = %v, want %v", res.Remaining, want)
	}

	// 3 台目には 1 本も発行していないこと。
	for _, c := range f.Calls() {
		if c.Options.Runner == "build01-7" {
			t.Errorf("失敗後も後続の台にコマンドを発行している: %v", c)
		}
	}
	// 成功した 1 台目のディレクトリは残す（FR-15）。
	if _, serr := os.Stat(filepath.Join(base, "build01-5", "config.sh")); serr != nil {
		t.Errorf("成功済みの runner を消している: %v", serr)
	}

	var se *setup.StepError
	if !errors.As(err, &se) {
		t.Fatalf("err の型 = %T, want *setup.StepError", err)
	}
	if se.Unit != "build01-6" || se.Phase != "登録" {
		t.Errorf("StepError = %+v", se)
	}
}

func TestApplyReportsProgressPerPhase(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	var got []string
	_, err := setup.Apply(context.Background(), setup.ApplyInput{
		Exec:     exec.NewFake(),
		Plan:     setuptest.AddPlanIn(t, base, 1),
		Token:    "TOKENTOKENTOKEN",
		TokenFor: nil,
		Tarball:  setuptest.MakeTarball(t),
		Drain:    nil,
		Progress: func(p setup.Progress) {
			got = append(got, p.Phase)
			if p.Total != 1 {
				t.Errorf("Total = %d, want 1", p.Total)
			}
		},
	})
	if err != nil {
		t.Fatalf("err = %v", err)
	}

	want := []string{"ディレクトリ作成", "展開", "登録", "サービス登録", "起動", "完了"}
	if !slices.Equal(got, want) {
		t.Errorf("進捗:\n got: %v\nwant: %v", got, want)
	}
}

// 終了要求を受けたら、着手していない台には一切手を付けない
// （docs/architecture/security.md「context でキャンセルできる」）。
func TestApplyStopsOnCancelAndLeavesRemainingUntouched(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	ctx, cancel := context.WithCancel(t.Context())

	f := exec.NewFake()
	// 1 台目の登録が済んだところで終了要求が来た状況を作る。
	f.SetFunc(func(name string, _ []string) (exec.Result, error) {
		if name == "./config.sh" {
			cancel()
		}
		return exec.Result{Stdout: nil, Stderr: nil, ExitCode: 0}, nil
	})

	res, err := setup.Apply(ctx, setup.ApplyInput{
		Exec:     f,
		Plan:     setuptest.AddPlanIn(t, base, 3),
		Token:    "TOKENTOKENTOKEN",
		TokenFor: nil,
		Tarball:  setuptest.MakeTarball(t),
		Drain:    nil,
		Progress: nil,
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if len(res.Succeeded) != 0 {
		t.Errorf("成功 = %v, want 空（1 台目は途中で止まっている）", res.Succeeded)
	}
	if res.Failed != "build01-5" {
		t.Errorf("中断した台 = %q, want build01-5", res.Failed)
	}
	if want := []string{"build01-6", "build01-7"}; !slices.Equal(res.Remaining, want) {
		t.Errorf("未着手 = %v, want %v", res.Remaining, want)
	}

	for _, c := range f.Calls() {
		if c.Options.Runner != "build01-5" {
			t.Errorf("未着手の台にコマンドを発行している: %v", c)
		}
	}
	for _, name := range []string{"build01-6", "build01-7"} {
		if _, serr := os.Stat(filepath.Join(base, name)); !errors.Is(serr, os.ErrNotExist) {
			t.Errorf("未着手の台のディレクトリを作っている: %s (%v)", name, serr)
		}
	}
}

func TestApplyValidatesInput(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	plan := setuptest.AddPlanIn(t, base, 1)

	tests := map[string]struct {
		in   setup.ApplyInput
		want error
	}{
		"Executor が無い": {
			setup.ApplyInput{
				Exec: nil, Plan: plan, Token: "t", TokenFor: nil, Tarball: "x",
				Drain: nil, Progress: nil,
			},
			setup.ErrNoExecutor,
		},
		"対象が無い": {
			setup.ApplyInput{
				Exec: exec.NewFake(), Plan: setup.Plan{}, Token: "t", TokenFor: nil, Tarball: "x",
				Drain: nil, Progress: nil,
			},
			setup.ErrNoTargets,
		},
		"トークンが無い": {
			setup.ApplyInput{
				Exec: exec.NewFake(), Plan: plan, Token: "", TokenFor: nil, Tarball: "x",
				Drain: nil, Progress: nil,
			},
			setup.ErrTokenRequired,
		},
		"tarball が無い": {
			setup.ApplyInput{
				Exec: exec.NewFake(), Plan: plan, Token: "t", TokenFor: nil, Tarball: "",
				Drain: nil, Progress: nil,
			},
			setup.ErrTarballRequired,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			res, err := setup.Apply(context.Background(), tt.in)
			if !errors.Is(err, tt.want) {
				t.Errorf("err = %v, want %v", err, tt.want)
			}
			if len(res.Succeeded) != 0 {
				t.Errorf("検証で落ちたのに成功が記録されている: %v", res.Succeeded)
			}
		})
	}
}
