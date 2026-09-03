package job_test

import (
	"context"
	"slices"
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/exec"
	"github.com/ousiassllc/gsr-helper/internal/runner/scope"
	"github.com/ousiassllc/gsr-helper/internal/setup"
	"github.com/ousiassllc/gsr-helper/internal/setup/job"
)

// tokenArg は config.sh の引数列から --token に渡された実値を取り出す。
//
// 監査ログのマスク（exec/mask の段 2）が効いているかは「発行時に何の値が
// 使われたか」を知らないと確かめられないため、計画側の目印ではなく実際の
// 引数から取る。
func tokenArg(args []string) string {
	i := slices.Index(args, "--token")
	if i < 0 || i+1 >= len(args) {
		return ""
	}
	return args[i+1]
}

// AC-13: 短命トークンは、それを載せたコマンドを発行している最中にマスク対象へ
// 入っていること。
//
// 実行後に Add の痕跡を見るだけでは足りない。マスクは Run の最中に
// gh.Secrets.Values() を引くので、発行より後に登録されても監査ログには平文が
// 残る。そのため config.sh の実行中（= Fake の中）から Secrets を覗く。
func TestRunArmsMaskingWhileShortTokenIsIssued(t *testing.T) {
	t.Parallel()

	f := exec.NewFake()
	d, _ := api(t, f)
	log := new(fetchLog)
	d.Fetch = log.fetch(t)

	// Apply は逐次実行なのでこのコールバックと検証は同じ goroutine で動く。
	var usedToken string
	masked := false
	f.SetFunc(func(name string, args []string) (exec.Result, error) {
		if name == "./config.sh" {
			usedToken = tokenArg(args)
			masked = slices.Contains(d.Secrets.Values(), usedToken)
		}
		return exec.Result{Stdout: nil, Stderr: nil, ExitCode: 0}, nil
	})

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

	if _, err = job.Run(context.Background(), job.Input{
		Deps: d, Plan: plan, Scope: sc, Drain: nil, Progress: nil,
	}); err != nil {
		t.Fatalf("err = %v", err)
	}

	// API が返した実トークンがそのまま渡っていること（目印のままだと以下が空振りする）。
	const want = "TOK/orgs/foo/actions/runners/registration-token"
	if usedToken != want {
		t.Fatalf("登録に使ったトークン = %q, want %q", usedToken, want)
	}
	if !masked {
		t.Error("発行の時点で短命トークンがマスク対象に入っていない（監査ログに平文が残る）")
	}

	// 使い終わったら参照を破棄する（security.md「保持と出力」）。
	if slices.Contains(d.Secrets.Values(), usedToken) {
		t.Errorf("短命トークンがマスク対象に残っている: %q", usedToken)
	}
}
