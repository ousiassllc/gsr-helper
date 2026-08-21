package runner

import (
	"fmt"
	"net/url"
	"strings"
)

// ScopeKind は runner がどのレベルに登録されているかを表す。
type ScopeKind int

const (
	ScopeUnknown ScopeKind = iota
	ScopeRepo
	ScopeOrg
	ScopeEnterprise
)

// Scope は runner の登録先。
type Scope struct {
	Kind  ScopeKind
	Owner string // org 名 / repo の owner / enterprise slug
	Repo  string // Kind が ScopeRepo のときのみ設定される
}

// String はテーブル表示用の短い表記を返す。
func (s Scope) String() string {
	switch s.Kind {
	case ScopeRepo:
		return s.Owner + "/" + s.Repo
	case ScopeOrg:
		return "org:" + s.Owner
	case ScopeEnterprise:
		return "ent:" + s.Owner
	default:
		return "-"
	}
}

// ParseScope は .runner の gitHubUrl からスコープを判定する。
//
// 想定する形式:
//
//	https://github.com/owner/repo    → repo
//	https://github.com/orgs/foo      → org
//	https://github.com/foo           → org
//	https://github.com/enterprises/e → enterprise
func ParseScope(githubURL string) (Scope, error) {
	raw := strings.TrimSpace(githubURL)
	if raw == "" {
		return Scope{}, fmt.Errorf("gitHubUrl が空です")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return Scope{}, fmt.Errorf("gitHubUrl の解析に失敗しました: %w", err)
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
	case parts[0] == "orgs" && len(parts) >= 2:
		return Scope{Kind: ScopeOrg, Owner: parts[1]}, nil
	case parts[0] == "enterprises" && len(parts) >= 2:
		return Scope{Kind: ScopeEnterprise, Owner: parts[1]}, nil
	case len(parts) >= 2:
		return Scope{Kind: ScopeRepo, Owner: parts[0], Repo: parts[1]}, nil
	default:
		// パス要素が 1 つだけなら org レベル登録。
		return Scope{Kind: ScopeOrg, Owner: parts[0]}, nil
	}
}
