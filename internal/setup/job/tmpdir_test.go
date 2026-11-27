package job_test

// t.Setenv で TMPDIR を差し替えるため、このファイルのテストは t.Parallel() を付けない。

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/exec"
	"github.com/ousiassllc/gsr-helper/internal/runner"
	"github.com/ousiassllc/gsr-helper/internal/runner/scope"
	"github.com/ousiassllc/gsr-helper/internal/setup"
	"github.com/ousiassllc/gsr-helper/internal/setup/job"
	"github.com/ousiassllc/gsr-helper/internal/setup/tarball"
)

// addPlan は tarball を要る追加計画を作る。
func addPlan(t *testing.T, sc scope.Scope) setup.Plan {
	t.Helper()

	plan, err := setup.PlanAdd(setup.AddSpec{
		URL: "https://github.com/orgs/foo", Scope: sc,
		NamePrefix: "build01", Count: 1, StartIndex: 1, Names: nil,
		Labels: nil, WorkDir: "_work", RunnerGroup: "", Ephemeral: false,
		DisableUpdate: false, InstallBase: t.TempDir(), RunAsUser: "", Version: "2.311.0",
		Existing: nil, Busy: nil,
	})
	if err != nil {
		t.Fatalf("PlanAdd: %v", err)
	}
	return plan
}

// assertNoRunnerTmp は tmp 配下に gsr-helper-runner-* が残っていないことを確かめる。
func assertNoRunnerTmp(t *testing.T, tmp string) {
	t.Helper()

	left, err := filepath.Glob(filepath.Join(tmp, "gsr-helper-runner-*"))
	if err != nil {
		t.Fatalf("Glob: %v", err)
	}
	if len(left) != 0 {
		t.Errorf("一時ディレクトリが残っている: %v", left)
	}
}

// 取得に成功しても失敗しても、キャンセルされても、Run は tarball の置き場を残さない（#207）。
func TestRunRemovesTarballDir(t *testing.T) {
	sc := scope.Scope{Kind: scope.Org, Owner: "foo", Repo: ""}

	tests := map[string]func(t *testing.T, d *job.Deps, cancel context.CancelFunc){
		"成功": func(t *testing.T, d *job.Deps, _ context.CancelFunc) {
			t.Helper()
			d.Fetch = new(fetchLog).fetch(t)
		},
		"取得失敗": func(t *testing.T, d *job.Deps, _ context.CancelFunc) {
			t.Helper()
			d.Fetch = func(_ context.Context, _ tarball.Info, dir string) (string, error) {
				// 取得途中のファイルが残った場合も含めて消えること。
				if err := os.WriteFile(filepath.Join(dir, "partial"), []byte("x"), 0o600); err != nil {
					t.Errorf("準備に失敗: %v", err)
				}
				return "", os.ErrPermission
			}
		},
		"キャンセル": func(t *testing.T, d *job.Deps, cancel context.CancelFunc) {
			t.Helper()
			d.Fetch = func(ctx context.Context, _ tarball.Info, _ string) (string, error) {
				cancel()
				return "", ctx.Err()
			}
		},
	}

	for name, arrange := range tests {
		t.Run(name, func(t *testing.T) {
			tmp := t.TempDir()
			t.Setenv("TMPDIR", tmp)

			d, _ := api(t, exec.NewFake())
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			arrange(t, &d, cancel)

			_, err := job.Run(ctx, job.Input{
				Deps: d, Plan: addPlan(t, sc), Scope: sc, Drain: nil, Progress: nil,
			})
			if name == "成功" && err != nil {
				t.Fatalf("err = %v", err)
			}
			if name == "キャンセル" && !errors.Is(err, context.Canceled) {
				t.Errorf("err = %v, want context.Canceled", err)
			}

			assertNoRunnerTmp(t, tmp)
		})
	}
}

// tarball が要らない計画では一時ディレクトリを作らない。
func TestRunCreatesNoTarballDirWhenNotNeeded(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)

	d, _ := api(t, exec.NewFake())
	sc := scope.Scope{Kind: scope.Org, Owner: "foo", Repo: ""}

	plan, err := setup.PlanRemove(setup.RemoveSpec{Runners: []runner.Runner{
		testRunner("a", t.TempDir(), "", sc, false),
	}})
	if err != nil {
		t.Fatalf("PlanRemove: %v", err)
	}

	if _, err = job.Run(context.Background(), job.Input{
		Deps: d, Plan: plan, Scope: sc, Drain: nil, Progress: nil,
	}); err != nil {
		t.Fatalf("err = %v", err)
	}
	assertNoRunnerTmp(t, tmp)
}
