package authz_test

import (
	"strings"
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/appconfig"
	"github.com/ousiassllc/gsr-helper/internal/doctor/check"
	"github.com/ousiassllc/gsr-helper/internal/runner"
	"github.com/ousiassllc/gsr-helper/internal/runner/scope"
)

// org レベルの runner には admin:org が要る。gh auth login の既定では付かない。
func TestScopesDetectsMissingAdminOrg(t *testing.T) {
	t.Parallel()

	in := scopesInput(t, "repo, workflow, read:org", true, []runner.Runner{orgRunner("build01")})

	got := only(t, run(t, "authz.scopes", in))
	if got.Status != check.Warn {
		t.Fatalf("Status = %v, want %v（Detail: %s）", got.Status, check.Warn, got.Detail)
	}
	if !strings.Contains(got.Remedy, "gh auth refresh -h github.com -s admin:org") {
		t.Errorf("Remedy に gh auth refresh が無い: %s", got.Remedy)
	}
}

// 足りていれば OK。
func TestScopesSatisfied(t *testing.T) {
	t.Parallel()

	in := scopesInput(t, "repo, admin:org", true, []runner.Runner{orgRunner("build01")})

	if got := only(t, run(t, "authz.scopes", in)).Status; got != check.OK {
		t.Errorf("Status = %v, want %v", got, check.OK)
	}
}

// fine-grained PAT はスコープを持たない。**「不足」と扱ってはならない。**
func TestScopesFineGrainedTokenIsSkipped(t *testing.T) {
	t.Parallel()

	in := scopesInput(t, "", false, []runner.Runner{orgRunner("build01")})

	got := only(t, run(t, "authz.scopes", in))
	if got.Status != check.Skip {
		t.Errorf("Status = %v, want %v（Detail: %s）", got.Status, check.Skip, got.Detail)
	}
}

// トークンが無ければ SKIP（FAIL と区別する）。
func TestScopesWithoutToken(t *testing.T) {
	t.Parallel()

	in := check.Input{
		Runners: []runner.Runner{orgRunner("build01")},
		Caps:    appconfig.Caps{GitHubToken: false},
	}
	if got := only(t, run(t, "authz.scopes", in)).Status; got != check.Skip {
		t.Errorf("Status = %v, want %v", got, check.Skip)
	}
}

// 登録先を判定できる runner が無ければ、必要なスコープが決まらないので SKIP。
func TestScopesWithoutDeterminableScope(t *testing.T) {
	t.Parallel()

	in := check.Input{
		Runners: []runner.Runner{newRunner("build01")},
		Caps:    appconfig.Caps{GitHubToken: true},
	}
	if got := only(t, run(t, "authz.scopes", in)).Status; got != check.Skip {
		t.Errorf("Status = %v, want %v", got, check.Skip)
	}
}

// 必要なスコープが複数あれば行を分ける。並びは権限の強い順で固定する。
func TestScopesReturnsOneRowPerRequiredScope(t *testing.T) {
	t.Parallel()

	repo := newRunner("build02")
	repo.Scope = scope.Scope{Kind: scope.Repo, Owner: "acme", Repo: "app"}
	in := scopesInput(t, "workflow", true, []runner.Runner{orgRunner("build01"), repo})

	got := run(t, "authz.scopes", in)
	if len(got) != 2 {
		t.Fatalf("件数 = %d, want 2（%+v）", len(got), got)
	}
	if !strings.Contains(got[0].Summary, "admin:org") {
		t.Errorf("1 行目 = %q, want admin:org が先", got[0].Summary)
	}
	if !strings.Contains(got[1].Summary, "repo") {
		t.Errorf("2 行目 = %q, want repo", got[1].Summary)
	}
}
