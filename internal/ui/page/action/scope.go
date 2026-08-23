package action

import (
	"github.com/ousiassllc/gsr-helper/internal/gh"
	"github.com/ousiassllc/gsr-helper/internal/runner"
	"github.com/ousiassllc/gsr-helper/internal/runner/scope"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
)

// 保有スコープによる可否の判定（Issue #79。screens.md「無効な操作の表示」の 6 段目）。
//
// allow.go から分けているのは、1 ファイル 300 行の上限に収めるためと、判定の入力が
// 他の段（root / systemd / 認証 / ジョブ実行中）と違って**共有状態から届く非同期の値**
// だからである。

// scopeLevelName は登録先の言い換え。理由の文言に使う（screens.md の 6 段目
// `org レベルの操作には admin:org が必要です`）。
func scopeLevelName(sc scope.Scope) string {
	switch sc.Kind {
	case scope.Repo:
		return "repo"
	case scope.Org:
		return "org"
	case scope.Enterprise:
		return "enterprise"
	case scope.Unknown:
		return ""
	default:
		return ""
	}
}

// missingScope は保有スコープが足りない場合に理由を返す。足りていれば空文字。
//
// 判定は次の 3 つがそろったときだけ行う。**塞ぐ側ではなく通す側に倒す**のがこの
// 関数の要点である（screens.md「無効な操作の表示」の 6 段目）。
//
//   - 取得を終えている（ScopeState.Known）。判定前に塞ぐと、権限の足りている
//     トークンで起動直後だけ操作できなくなる。取得に失敗した場合も Known は偽の
//     ままなので、ここで通る
//   - スコープという概念を持つトークンである（gh.Scopes.Classic）。fine-grained PAT と
//     GitHub App のトークンは X-OAuth-Scopes を返さない。「スコープが無い」と扱うと、
//     権限が十分なトークンを誤って塞ぐ
//   - 登録先から必要なスコープが決まる。Runners タブに runner が 1 台も無い場合や
//     Setup タブのメニューのように対象が定まらない場合は判定材料が無いので塞がない
func missingScope(r runner.Runner, sc page.ScopeState) string {
	if !sc.Known || !sc.Scopes.Classic {
		return ""
	}
	need := gh.RequiredScope(r.Scope)
	if need == "" || sc.Scopes.Has(need) {
		return ""
	}
	level := scopeLevelName(r.Scope)
	return level + " レベルの操作には " + need +
		" が必要です（gh auth refresh -h github.com -s " + need + "）"
}
