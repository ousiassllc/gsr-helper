package authz

import (
	"context"
	"strings"

	"github.com/ousiassllc/gsr-helper/internal/doctor/check"
	"github.com/ousiassllc/gsr-helper/internal/gh"
)

// scopes は GitHub トークンの保有スコープが runner の登録先に足りているかを
// 判定する。
//
// `gh auth login` の既定スコープには admin:org が含まれないため、org レベルの
// runner を管理しようとすると 403 になる（external-interfaces.md「必要な
// トークンスコープ」）。この判定が無い間、権限不足は API を呼んだ時点まで
// 分からなかった。
//
// **API を叩くのは 1 回だけである。** 保有スコープはトークンごとの属性であり
// runner の数とは関係しない。runner ごとに引くとレート制限を台数ぶん消費する。
type scopes struct{}

func (scopes) ID() string       { return "authz.scopes" }
func (scopes) Category() string { return check.CatAuthz }
func (scopes) Startup() bool    { return false }

// Run は必要なスコープごとに 1 行を返す。
func (c scopes) Run(ctx context.Context, in check.Input) []check.Result {
	if !in.Caps.GitHubToken {
		return []check.Result{check.Skipped(c, "トークンの保有スコープ",
			"GitHub のトークンを取得できないため保有スコープを確認していません。")}
	}

	needs := requiredScopes(in)
	if len(needs) == 0 {
		return []check.Result{check.Skipped(c, "トークンの保有スコープ",
			"登録先を判定できる runner が無いため、必要なスコープが決まりません。")}
	}

	held, err := c.fetch(ctx, in)
	if err != nil {
		return []check.Result{check.Of(c, check.Result{
			Status:  check.Warn,
			Summary: "トークンの保有スコープを確認できない",
			Detail:  "保有スコープの取得に失敗しました: " + err.Error(),
			Impact:  "権限不足があっても、実際に API を呼ぶまで分かりません。",
			Remedy:  "gh auth status でトークンの状態を確認してください。",
		})}
	}

	// fine-grained PAT と GitHub App のトークンは X-OAuth-Scopes を返さない。
	// 「スコープが無い」と扱うと、権限が十分なトークンに対して誤って警告する。
	if !held.Classic {
		return []check.Result{check.Skipped(c, "トークンの保有スコープ",
			"fine-grained PAT または GitHub App のトークンで、保有スコープの概念がありません"+
				"（権限は API を呼んだ時点で判定されます）。")}
	}

	out := make([]check.Result, 0, len(needs))
	for _, need := range needs {
		out = append(out, c.judge(held, need))
	}
	return out
}

// fetch は保有スコープを 1 回だけ取得する。
func (c scopes) fetch(ctx context.Context, in check.Input) (gh.Scopes, error) {
	client, err := in.Client(ctx)
	if err != nil {
		return gh.Scopes{}, err
	}
	return client.TokenScopes(ctx)
}

// judge は必要なスコープ 1 つぶんの判定を返す。
func (c scopes) judge(held gh.Scopes, need string) check.Result {
	if held.Has(need) {
		return check.Of(c, check.Result{
			Status:  check.OK,
			Summary: "トークンに " + need + " がある",
			Detail:  "保有スコープ: " + heldList(held),
		})
	}
	return check.Of(c, check.Result{
		Status:  check.Warn,
		Summary: "トークンに " + need + " がない",
		Detail:  "保有スコープ: " + heldList(held),
		Impact: "この登録先の runner を追加・削除・一覧する操作が HTTP 403 で失敗します" +
			"（gh auth login の既定スコープに " + need + " は含まれません）。",
		Remedy: "gh auth refresh -h github.com -s " + need,
	})
}

// requiredScopes は検出済みの runner の登録先から、必要なスコープを重複なく返す。
//
// 並びは runner の検出順に依存させない。実行のたびに行が入れ替わるためである。
func requiredScopes(in check.Input) []string {
	seen := make(map[string]bool, 3)
	for _, r := range in.Runners {
		if need := gh.RequiredScope(r.Scope); need != "" {
			seen[need] = true
		}
	}
	// 権限の強い順に並べる。不足しているときに影響の大きいものを先に読ませる。
	out := make([]string, 0, len(seen))
	for _, need := range []string{"admin:enterprise", "admin:org", "repo"} {
		if seen[need] {
			out = append(out, need)
		}
	}
	return out
}

// heldList は保有スコープを画面に出す表記にする。
func heldList(held gh.Scopes) string {
	if len(held.Held) == 0 {
		return "（1 つもありません）"
	}
	return strings.Join(held.Held, ", ")
}
