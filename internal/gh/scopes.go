package gh

import (
	"context"
	"net/http"
	"strings"

	"github.com/ousiassllc/gsr-helper/internal/runner/scope"
)

// scopeHeader は保有スコープが載る応答ヘッダ（external-interfaces.md）。
const scopeHeader = "X-OAuth-Scopes"

// scopesPath は保有スコープを引くために叩くエンドポイント。
//
// /rate_limit を選んだのは、**レート制限を消費しない**唯一の endpoint だから
// である（GitHub の仕様）。診断は繰り返し実行されるものなので、確認そのものが
// 制限を削る形にはしない。応答ヘッダは他の endpoint と同じものが載る。
const scopesPath = "rate_limit"

// Scopes はトークンの保有スコープ。
//
// Classic を別に持つのは、**「スコープが無い」と「スコープという概念が無い」を
// 区別する**ためである。fine-grained PAT と GitHub App のトークンは
// X-OAuth-Scopes を返さない。両者を同じ「空」で扱うと、権限が十分な
// fine-grained PAT に対して「admin:org がありません」と誤って警告する。
type Scopes struct {
	// Held は保有しているスコープ。Classic が偽なら空。
	Held []string
	// Classic は X-OAuth-Scopes が返ったか（classic PAT / OAuth トークン）。
	Classic bool
}

// Has は need を満たすスコープを持っているかを返す。
//
// GitHub のスコープは admin:x ⊃ write:x ⊃ read:x の包含関係を持つ。
// admin:org を持つトークンは read:org を要する操作を行えるので、
// 文字列の一致だけで判定すると持っている権限を「無い」と誤る。
func (s Scopes) Has(need string) bool {
	if need == "" {
		return true
	}
	for _, held := range s.Held {
		if held == need || covers(held, need) {
			return true
		}
	}
	return false
}

// covers は held が need を含むかを返す。
func covers(held, need string) bool {
	// repo は public_repo などの下位を含む。
	if held == "repo" && strings.HasPrefix(need, "public_repo") {
		return true
	}
	hLevel, hName, hOK := strings.Cut(held, ":")
	nLevel, nName, nOK := strings.Cut(need, ":")
	if !hOK || !nOK || hName != nName {
		return false
	}
	return scopeLevel(hLevel) >= scopeLevel(nLevel)
}

// scopeLevel は admin / write / read の強さを返す。未知の接頭辞は -1。
func scopeLevel(prefix string) int {
	switch prefix {
	case "read":
		return 1
	case "write":
		return 2
	case "admin":
		return 3
	default:
		return -1
	}
}

// TokenScopes はトークンの保有スコープを返す（doctor の認証・権限チェック）。
//
// 判定は「操作したいスコープに対して権限が足りているか」に使う
// （external-interfaces.md の必要なトークンスコープ）。
func (c *Client) TokenScopes(ctx context.Context) (Scopes, error) {
	resp, err := c.do(ctx, http.MethodGet, scopesPath, nil)
	if err != nil {
		return Scopes{Held: nil, Classic: false},
			wrap("token_scopes", scope.Scope{Kind: scope.Unknown, Owner: "", Repo: ""}, resp, err)
	}
	if resp == nil || resp.Response == nil {
		return Scopes{Held: nil, Classic: false}, nil
	}

	raw, ok := resp.Header[http.CanonicalHeaderKey(scopeHeader)]
	if !ok {
		// fine-grained PAT / GitHub App。スコープという形の権限を持たない。
		return Scopes{Held: nil, Classic: false}, nil
	}
	return Scopes{Held: splitScopes(strings.Join(raw, ",")), Classic: true}, nil
}

// splitScopes は "repo, admin:org" 形式の値を分解する。
//
// ヘッダが空文字（スコープを 1 つも持たないトークン）の場合も Classic は真に
// なる。空の要素を落とすので、戻りは長さ 0 のスライスになる。
func splitScopes(raw string) []string {
	out := make([]string, 0, strings.Count(raw, ",")+1)
	for _, s := range strings.Split(raw, ",") {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// RequiredScope は対象スコープの runner を管理するのに必要な classic PAT の
// スコープを返す。判定できない場合は空を返す。
//
// doctor が同じ表を持たずに済むよう公開する（表が 2 箇所にあると、必要な
// スコープを変えたときに API の 403 と診断の判定が食い違う）。
func RequiredScope(sc scope.Scope) string { return requiredScope(sc) }
