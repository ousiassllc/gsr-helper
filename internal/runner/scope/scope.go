// Package scope は runner の登録先（repo / org / enterprise）を .runner の
// gitHubUrl から判定する。
//
// runner から分離しているのは次の 2 点による。
//   - URL からのスコープ判定はホスト走査・/proc・systemd と無関係な純粋な
//     文字列処理であり、責務として独立している。
//   - Scope は GitHub API のパス生成にも使う（docs/architecture/data-model.md）。
//     runner の下位に置くことで、将来 internal/gh が internal/runner 全体を
//     import せずに済む。
package scope

import (
	"fmt"
	"net/url"
	"strings"
)

// Kind は runner がどのレベルに登録されているかを表す。
type Kind int

// Kind の取り得る値。
const (
	Unknown Kind = iota
	Repo
	Org
	Enterprise
)

// Scope は runner の登録先。
type Scope struct {
	Kind  Kind
	Owner string // org 名 / repo の owner / enterprise slug
	Repo  string // Kind が Repo のときのみ設定される
}

// String はテーブル表示用の短い表記を返す。
func (s Scope) String() string {
	switch s.Kind {
	case Repo:
		return s.Owner + "/" + s.Repo
	case Org:
		return "org:" + s.Owner
	case Enterprise:
		return "ent:" + s.Owner
	default:
		return "-"
	}
}

// Parse は .runner の gitHubUrl からスコープを判定する。
//
// 想定する形式:
//
//	https://github.com/owner/repo    → repo
//	https://github.com/orgs/foo      → org
//	https://github.com/foo           → org
//	https://github.com/enterprises/e → enterprise
func Parse(githubURL string) (Scope, error) {
	raw := strings.TrimSpace(githubURL)
	if raw == "" {
		return Scope{}, fmt.Errorf("gitHubUrl が空です")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return Scope{}, fmt.Errorf("gitHubUrl の解析に失敗しました: %w", err)
	}
	// ホストが無いものは URL として扱わない。スキームを欠いた
	// "github.com/foo/bar" や URL でない文字列は全体がパスとして解釈され、
	// ホスト名や無関係な単語が owner になってしまう。
	// GHES の https://ghe.example.com/orgs/foo はホストが入るため影響しない。
	if u.Host == "" {
		return Scope{}, fmt.Errorf("gitHubUrl がホストを含む URL ではありません: %s", raw)
	}

	var parts []string
	for _, p := range strings.Split(u.Path, "/") {
		if p != "" {
			parts = append(parts, p)
		}
	}

	switch {
	case len(parts) == 0:
		return Scope{}, fmt.Errorf("gitHubUrl にスコープが含まれていません: %s", raw)
	case parts[0] == "orgs":
		// orgs / enterprises は接頭辞であって owner ではない。
		// 後続要素が無ければスコープを決められないのでエラーにする。
		if len(parts) < 2 {
			return Scope{}, fmt.Errorf("gitHubUrl に org 名が含まれていません: %s", raw)
		}
		return Scope{Kind: Org, Owner: parts[1]}, nil
	case parts[0] == "enterprises":
		if len(parts) < 2 {
			return Scope{}, fmt.Errorf("gitHubUrl に enterprise 名が含まれていません: %s", raw)
		}
		return Scope{Kind: Enterprise, Owner: parts[1]}, nil
	case len(parts) >= 2:
		return Scope{Kind: Repo, Owner: parts[0], Repo: parts[1]}, nil
	default:
		// パス要素が 1 つだけなら org レベル登録。
		return Scope{Kind: Org, Owner: parts[0]}, nil
	}
}
