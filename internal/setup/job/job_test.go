package job_test

import (
	"context"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/exec"
	"github.com/ousiassllc/gsr-helper/internal/runner"
	"github.com/ousiassllc/gsr-helper/internal/runner/scope"
	"github.com/ousiassllc/gsr-helper/internal/setup"
	"github.com/ousiassllc/gsr-helper/internal/setup/job"
	"github.com/ousiassllc/gsr-helper/internal/setup/tarball"
)

func TestLatestVersion(t *testing.T) {
	t.Parallel()

	d, paths := api(t, exec.NewFake())
	got, err := job.LatestVersion(context.Background(), d)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if got != "2.311.0" {
		t.Errorf("バージョン = %q, want 2.311.0", got)
	}
	if !slices.Contains(*paths, "/repos/actions/runner/releases/latest") {
		t.Errorf("最新版のエンドポイントを叩いていない: %v", *paths)
	}
}

func TestRunReusesOneRegistrationTokenForBulkAdd(t *testing.T) {
	t.Parallel()

	f := exec.NewFake()
	d, paths := api(t, f)
	var fetched int32
	d.Fetch = fakeFetch(t, &fetched)

	base := t.TempDir()
	plan, err := setup.PlanAdd(setup.AddSpec{
		URL:        "https://github.com/orgs/foo",
		Scope:      scope.Scope{Kind: scope.Org, Owner: "foo", Repo: ""},
		NamePrefix: "build01", Count: 3, StartIndex: 1, Names: nil,
		Labels: nil, WorkDir: "_work", RunnerGroup: "", Ephemeral: false,
		DisableUpdate: false, InstallBase: base, RunAsUser: "", Version: "2.311.0",
		Existing: nil, Busy: nil,
	})
	if err != nil {
		t.Fatalf("PlanAdd: %v", err)
	}

	res, err := job.Run(context.Background(), job.Input{
		Deps: d, Plan: plan,
		Scope:    scope.Scope{Kind: scope.Org, Owner: "foo", Repo: ""},
		Drain:    nil,
		Progress: nil,
	})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if !res.OK() {
		t.Fatalf("Result = %+v", res)
	}

	// FR-14: 有効期限内なら 1 つのトークンを台数分の登録に使い回す。
	if n := count(*paths, "/registration-token"); n != 1 {
		t.Errorf("registration token の取得回数 = %d, want 1（台数分で使い回す）: %v", n, *paths)
	}
	// FR-13: 1 回の取得を各ディレクトリへ展開して使い回す。
	if fetched != 1 {
		t.Errorf("tarball の取得回数 = %d, want 1", fetched)
	}

	// 3 台とも同じトークンで登録している。
	for _, line := range issued(f) {
		if strings.Contains(line, "--token") && !strings.Contains(line, "TOK/orgs/foo") {
			t.Errorf("想定外のトークンで登録している: %s", line)
		}
	}
}

func TestRunTakesOneRemoveTokenPerScope(t *testing.T) {
	t.Parallel()

	f := exec.NewFake()
	d, paths := api(t, f)

	repoSc := scope.Scope{Kind: scope.Repo, Owner: "foo", Repo: "bar"}
	orgSc := scope.Scope{Kind: scope.Org, Owner: "foo", Repo: ""}
	plan, err := setup.PlanRemove(setup.RemoveSpec{Runners: []runner.Runner{
		testRunner("a", t.TempDir(), "a.service", repoSc, false),
		testRunner("b", t.TempDir(), "b.service", orgSc, false),
		testRunner("c", t.TempDir(), "c.service", repoSc, false),
	}})
	if err != nil {
		t.Fatalf("PlanRemove: %v", err)
	}

	if _, err = job.Run(context.Background(), job.Input{
		Deps: d, Plan: plan, Scope: repoSc, Drain: nil, Progress: nil,
	}); err != nil {
		t.Fatalf("err = %v", err)
	}

	// スコープごとに 1 本ずつ。3 台でも repo と org の 2 本で足りる。
	if n := count(*paths, "/remove-token"); n != 2 {
		t.Errorf("remove token の取得回数 = %d, want 2（スコープごとに 1 本）: %v", n, *paths)
	}
	if !slices.Contains(*paths, "/repos/foo/bar/actions/runners/remove-token") {
		t.Errorf("repo スコープの remove token を取っていない: %v", *paths)
	}
	if !slices.Contains(*paths, "/orgs/foo/actions/runners/remove-token") {
		t.Errorf("org スコープの remove token を取っていない: %v", *paths)
	}
}

func TestRunForgetsTokensAfterwards(t *testing.T) {
	t.Parallel()

	f := exec.NewFake()
	d, _ := api(t, f)
	sc := scope.Scope{Kind: scope.Org, Owner: "foo", Repo: ""}

	plan, err := setup.PlanRemove(setup.RemoveSpec{
		Runners: []runner.Runner{testRunner("a", t.TempDir(), "", sc, false)},
	})
	if err != nil {
		t.Fatalf("PlanRemove: %v", err)
	}

	if _, err = job.Run(context.Background(), job.Input{
		Deps: d, Plan: plan, Scope: sc, Drain: nil, Progress: nil,
	}); err != nil {
		t.Fatalf("err = %v", err)
	}

	// 短命トークンは使い終わったら参照を破棄する（security.md「保持と出力」）。
	for _, v := range d.Secrets.Values() {
		if strings.HasPrefix(v, "TOK") {
			t.Errorf("短命トークンがマスク対象に残っている: %q", v)
		}
	}
}

func TestRunReportsPreparationBeforeUnits(t *testing.T) {
	t.Parallel()

	f := exec.NewFake()
	d, _ := api(t, f)
	var fetched int32
	d.Fetch = fakeFetch(t, &fetched)

	sc := scope.Scope{Kind: scope.Org, Owner: "foo", Repo: ""}
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

	var phases []string
	if _, err = job.Run(context.Background(), job.Input{
		Deps: d, Plan: plan, Scope: sc, Drain: nil,
		Progress: func(p setup.Progress) { phases = append(phases, p.Phase) },
	}); err != nil {
		t.Fatalf("err = %v", err)
	}

	if len(phases) == 0 || phases[0] != job.PrepPhase {
		t.Errorf("最初の進捗 = %v, want %q が先頭", phases, job.PrepPhase)
	}
}

func TestRunReportsRemainingWhenPreparationFails(t *testing.T) {
	t.Parallel()

	f := exec.NewFake()
	d, _ := api(t, f)
	sc := scope.Scope{Kind: scope.Org, Owner: "foo", Repo: ""}
	plan, err := setup.PlanAdd(setup.AddSpec{
		URL: "https://github.com/orgs/foo", Scope: sc,
		NamePrefix: "build01", Count: 2, StartIndex: 1, Names: nil,
		Labels: nil, WorkDir: "_work", RunnerGroup: "", Ephemeral: false,
		DisableUpdate: false, InstallBase: t.TempDir(), RunAsUser: "", Version: "2.311.0",
		Existing: nil, Busy: nil,
	})
	if err != nil {
		t.Fatalf("PlanAdd: %v", err)
	}

	d.Fetch = func(context.Context, tarball.Info, string) (string, error) {
		return "", os.ErrPermission
	}

	res, err := job.Run(context.Background(), job.Input{
		Deps: d, Plan: plan, Scope: sc, Drain: nil, Progress: nil,
	})
	if err == nil {
		t.Fatal("err = nil, want エラー")
	}
	if len(res.Succeeded) != 0 {
		t.Errorf("成功 = %v, want 空", res.Succeeded)
	}
	if want := []string{"build01-1", "build01-2"}; !slices.Equal(res.Remaining, want) {
		t.Errorf("未実行 = %v, want %v", res.Remaining, want)
	}
	if len(f.Calls()) != 0 {
		t.Errorf("準備に失敗したのにコマンドを発行している: %v", issued(f))
	}
}
