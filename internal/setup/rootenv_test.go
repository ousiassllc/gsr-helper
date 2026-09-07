package setup_test

import (
	"context"
	"slices"
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/exec"
	"github.com/ousiassllc/gsr-helper/internal/runner"
	"github.com/ousiassllc/gsr-helper/internal/setup"
	"github.com/ousiassllc/gsr-helper/internal/setup/setuptest"
)

// configEnv は計画に載った config.sh の手順の Env を返す。
func configEnv(t *testing.T, p setup.Plan) []string {
	t.Helper()

	for _, u := range p.Units {
		for _, s := range u.Steps {
			if s.Name == "./config.sh" {
				return s.Env
			}
		}
	}
	t.Fatal("計画に config.sh の手順が無い")
	return nil
}

// root では config.sh の手順へ RUNNER_ALLOW_RUNASROOT を渡し、root でなければ渡さない。
//
// config.sh は uid 0 でこれが空だと `Must not run with sudo` で終了コード 1 になり、
// sudo 起動を前提とする gsr-helper では**追加も削除も必ず失敗する**（実機で発生）。
// 削除も見るのは、登録だけ通って解除が落ちる形にしないためである。
func TestPlansPassAllowRunAsRootOnlyWhenRoot(t *testing.T) {
	t.Parallel()

	add := func(root bool) (setup.Plan, error) {
		spec := setuptest.AddSpec()
		spec.Root = root
		return setup.PlanAdd(spec)
	}
	remove := func(root bool) (setup.Plan, error) {
		r := runner.Runner{Dir: "/opt/runners/build01-1", UnitName: ""}
		return setup.PlanRemove(setup.RemoveSpec{Runners: []runner.Runner{r}, Root: root})
	}

	for _, tt := range []struct {
		name string
		plan func(bool) (setup.Plan, error)
	}{{"追加", add}, {"削除", remove}} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			p, err := tt.plan(true)
			if err != nil {
				t.Fatalf("root で計画を組めない: %v", err)
			}
			if got := configEnv(t, p); !slices.Contains(got, setup.EnvAllowRunAsRoot) {
				t.Errorf("root の Env = %q, want %q を含む", got, setup.EnvAllowRunAsRoot)
			}

			if p, err = tt.plan(false); err != nil {
				t.Fatalf("非 root で計画を組めない: %v", err)
			}
			if got := configEnv(t, p); len(got) != 0 {
				t.Errorf("非 root の Env = %q, want 空", got)
			}
		})
	}
}

// Apply は Step.Env を実行時のオプションへ渡す。
//
// **この 1 本が欠けると、計画のテストだけ緑のまま root での登録が失敗し続ける。**
func TestApplyPassesStepEnvToExecutor(t *testing.T) {
	t.Parallel()

	spec := setuptest.AddSpec()
	spec.InstallBase, spec.Count, spec.Existing, spec.Root = t.TempDir(), 1, nil, true
	p, err := setup.PlanAdd(spec)
	if err != nil {
		t.Fatalf("PlanAdd: %v", err)
	}

	f := exec.NewFake()
	if _, err := setup.Apply(context.Background(), setup.ApplyInput{
		Exec: f, Plan: p, Token: "AREGISTRATIONTOKEN", TokenFor: nil,
		Tarball: setuptest.MakeTarball(t), Drain: nil, Progress: nil,
	}); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	for _, c := range f.Calls() {
		if c.Name != "./config.sh" {
			continue
		}
		if !slices.Contains(c.Options.Env, setup.EnvAllowRunAsRoot) {
			t.Errorf("Options.Env = %q, want %q を含む", c.Options.Env, setup.EnvAllowRunAsRoot)
		}
		return
	}
	t.Error("config.sh が実行されていない")
}
