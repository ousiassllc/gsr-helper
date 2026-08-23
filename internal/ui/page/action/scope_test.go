package action

import (
	"strings"
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/gh"
	"github.com/ousiassllc/gsr-helper/internal/runner"
	"github.com/ousiassllc/gsr-helper/internal/runner/scope"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
)

// スコープ不足の判定（Issue #79。screens.md「無効な操作の表示」の 6 段目）を検証する。
//
// **要点は塞ぐことではなく、塞ぎすぎないことである。** 判定前・取得失敗・スコープの
// 概念を持たないトークンでは、権限の足りているトークンの操作を奪ってはならない。

// scoped は登録先を差し替えた runner を返す。
func scoped(kind scope.Kind) runner.Runner {
	r := sampleRunner()
	r.Scope = scope.Scope{Kind: kind, Owner: "foo", Repo: "bar"}
	return r
}

// classic は保有スコープが分かっている classic PAT の状態を返す。
func classic(held ...string) page.ScopeState {
	return page.ScopeState{Scopes: gh.Scopes{Held: held, Classic: true}, Known: true}
}

// 保有スコープが足りない org の runner では n / D が塞がる。
func TestAllowBlocksAddAndDeleteWhenScopeIsMissing(t *testing.T) {
	st := classic("repo")
	r := scoped(scope.Org)

	for _, k := range []string{"n", "D"} {
		t.Run(k, func(t *testing.T) {
			ok, reason := testActions().withScopes(st).Allowed(k, r, fullCaps())
			if ok {
				t.Fatal("スコープが足りないのに操作できる")
			}
			if !strings.Contains(reason, "admin:org") {
				t.Errorf("理由 = %q, want admin:org を含む", reason)
			}
			if !strings.Contains(reason, "gh auth refresh") {
				t.Errorf("理由に是正の手順が無い: %q", reason)
			}
		})
	}
}

// u（バージョン更新）は塞がない。表が塞ぐと定めているのは n / D だけである。
func TestAllowDoesNotBlockUpdateOnMissingScope(t *testing.T) {
	if ok, reason := testActions().withScopes(classic("repo")).Allowed("u", scoped(scope.Org), fullCaps()); !ok {
		t.Errorf("u が塞がれている: %q", reason)
	}
}

// 塞がない条件を並べて確かめる。
func TestAllowDoesNotBlockWhenScopeIsUnknownOrSufficient(t *testing.T) {
	tests := []struct {
		name   string
		state  page.ScopeState
		runner runner.Runner
	}{
		{
			// 起動直後。判定前に塞ぐと、権限のあるトークンで一時的に操作できなくなる。
			name: "取得前は塞がない", state: page.ScopeState{}, runner: scoped(scope.Org),
		},
		{
			// 取得に失敗しても Known は偽のまま返る（ghscope.Start）。
			name:  "取得に失敗しても塞がない",
			state: page.ScopeState{Scopes: gh.Scopes{}, Known: false}, runner: scoped(scope.Org),
		},
		{
			// fine-grained PAT / GitHub App。X-OAuth-Scopes を返さないので、
			// 「スコープが無い」と扱うと権限の十分なトークンを誤って塞ぐ。
			name:   "スコープの概念を持たないトークンは塞がない",
			state:  page.ScopeState{Scopes: gh.Scopes{Classic: false}, Known: true},
			runner: scoped(scope.Org),
		},
		{
			name:  "必要なスコープを持っていれば塞がない",
			state: classic("admin:org"), runner: scoped(scope.Org),
		},
		{
			// admin:x ⊃ write:x ⊃ read:x の包含は gh.Scopes.Has が見る。
			name:  "上位のスコープでも足りていれば塞がない",
			state: classic("repo"), runner: scoped(scope.Repo),
		},
		{
			// 登録先が決まらなければ必要なスコープも決まらない
			// （Setup タブのメニューは対象を持たない runner で判定する）。
			name:  "登録先が判定できなければ塞がない",
			state: classic(), runner: runner.Runner{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, k := range []string{"n", "D"} {
				if ok, reason := testActions().withScopes(tt.state).Allowed(k, tt.runner, fullCaps()); !ok {
					t.Errorf("%s が塞がれている: %q", k, reason)
				}
			}
		})
	}
}

// 詳細画面の操作リストにも同じ理由が出る（フッタと食い違わせない）。
func TestChoicesCarryScopeReason(t *testing.T) {
	items := testActions().withScopes(classic("repo")).Choices(scoped(scope.Org), fullCaps())

	for _, c := range items {
		if c.ID != Delete.String() {
			continue
		}
		if c.Enabled {
			t.Fatal("操作リストで削除が有効になっている")
		}
		if !strings.Contains(c.Reason, "admin:org") {
			t.Errorf("操作リストの理由 = %q, want admin:org を含む", c.Reason)
		}
		return
	}
	t.Fatal("削除の項目が操作リストに無い")
}
