package setup_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/exec"
	"github.com/ousiassllc/gsr-helper/internal/runner"
	"github.com/ousiassllc/gsr-helper/internal/runner/scope"
	"github.com/ousiassllc/gsr-helper/internal/setup"
	"github.com/ousiassllc/gsr-helper/internal/setup/setuptest"
)

func TestApplyUsesPerUnitTokenForMixedScopes(t *testing.T) {
	t.Parallel()

	// 削除の対象が repo と org にまたがる場合、remove token はスコープごとに違う。
	repoRunner := setuptest.Runner("build01-3", "/opt/runners/build01-3", "u3.service", false, false)
	repoRunner.Scope = scope.Scope{Kind: scope.Repo, Owner: "foo", Repo: "bar"}
	orgRunner := setuptest.Runner("build01-4", "/opt/runners/build01-4", "u4.service", false, false)
	orgRunner.Scope = scope.Scope{Kind: scope.Org, Owner: "foo", Repo: ""}

	p, err := setup.PlanRemove(setup.RemoveSpec{Runners: []runner.Runner{repoRunner, orgRunner}})
	if err != nil {
		t.Fatalf("PlanRemove: %v", err)
	}

	f := exec.NewFake()
	res, err := setup.Apply(context.Background(), setup.ApplyInput{
		Exec:  f,
		Plan:  p,
		Token: "",
		TokenFor: func(_ context.Context, u setup.Unit) (string, error) {
			return "TOKEN-" + u.Runner.Scope.String(), nil
		},
		Tarball:  "",
		Drain:    nil,
		Progress: nil,
	})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if !res.OK() {
		t.Fatalf("Result = %+v", res)
	}

	want := []string{
		"./svc.sh stop", "./svc.sh uninstall", "./config.sh remove --token TOKEN-foo/bar",
		"./svc.sh stop", "./svc.sh uninstall", "./config.sh remove --token TOKEN-org:foo",
	}
	if got := setuptest.Issued(f); !slices.Equal(got, want) {
		t.Errorf("発行コマンド:\n got: %v\nwant: %v", got, want)
	}
}

func TestApplyFailsWhenPerUnitTokenIsEmpty(t *testing.T) {
	t.Parallel()

	r := setuptest.Runner("build01-3", "/opt/runners/build01-3", "", false, false)
	p, err := setup.PlanRemove(setup.RemoveSpec{Runners: []runner.Runner{r}})
	if err != nil {
		t.Fatalf("PlanRemove: %v", err)
	}

	_, err = setup.Apply(context.Background(), setup.ApplyInput{
		Exec:     exec.NewFake(),
		Plan:     p,
		Token:    "",
		TokenFor: func(context.Context, setup.Unit) (string, error) { return "", nil },
		Tarball:  "",
		Drain:    nil,
		Progress: nil,
	})
	if !errors.Is(err, setup.ErrTokenRequired) {
		t.Errorf("err = %v, want ErrTokenRequired", err)
	}
}

func TestApplyUsesDrainHookForUpdate(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	r := setuptest.Runner("build01-1", dir, "u1.service", true, false)
	p, err := setup.PlanUpdate(setup.UpdateSpec{Runners: []runner.Runner{r}, Version: "2.311.0"})
	if err != nil {
		t.Fatalf("PlanUpdate: %v", err)
	}

	// 更新後も残ることを確かめるため、保持対象のファイルを置いておく。
	// makeTarball は同名のエントリを別の中身で持っているので、上書きされれば分かる。
	kept := map[string]string{
		".runner": `{"agentName":"build01-1"}`,
		".env":    "HOST_ENV=1\n",
	}
	for name, body := range kept {
		if werr := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); werr != nil {
			t.Fatalf("準備に失敗: %v", werr)
		}
	}

	f := exec.NewFake()
	drained := 0
	res, err := setup.Apply(context.Background(), setup.ApplyInput{
		Exec:     f,
		Plan:     p,
		Token:    "",
		TokenFor: nil,
		Tarball:  setuptest.MakeTarball(t),
		Drain: func(context.Context, exec.Executor, runner.Runner) error {
			drained++
			return nil
		},
		Progress: nil,
	})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if !res.OK() {
		t.Fatalf("Result = %+v", res)
	}
	if drained != 1 {
		t.Errorf("ドレイン停止の回数 = %d, want 1", drained)
	}
	if want := []string{"./svc.sh start"}; !slices.Equal(setuptest.Issued(f), want) {
		t.Errorf("発行コマンド = %v, want %v", setuptest.Issued(f), want)
	}

	for name, want := range kept {
		body, rerr := os.ReadFile(filepath.Join(dir, name))
		if rerr != nil || string(body) != want {
			t.Errorf("%s が上書きされている（FR-21）: %q, %v", name, string(body), rerr)
		}
	}
}
