package molecule

import (
	"strconv"

	"github.com/ousiassllc/gsr-helper/internal/ui/atom"
	"github.com/ousiassllc/gsr-helper/internal/ui/token"
)

// appName はヘッダの先頭に出すツール名。
const appName = "gsr-helper"

// CapsView はヘッダに出す能力判定の結果。
//
// ディスク使用率は持たない。集計は internal/disk の担当であり、未実装の値を
// ヘッダに置くと「集計中」と「取得失敗」を区別できない表示になる。
type CapsView struct {
	Host       string // ホスト名
	Root       bool   // root 権限があるか
	Systemd    bool   // systemctl が使えるか
	GitHubUser string // gh の認証ユーザー
	HasToken   bool   // gh が認証済みか
	// HostReq は起動時のジョブ実行の前提チェック（FR-44）で見つかった不備の件数。
	// 0 なら出さない。**判定前も 0 である**ため、確定するまで警告は現れない。
	HostReq int
}

// CapsBar はヘッダ行を返す。
//
// 依存する操作が使えないことをここで示す（root が無ければ read-only、
// gh が未認証なら「gh: 未認証」）。理由の詳細はフッタの KeyBar が出す。
func CapsBar(v CapsView, width int, s token.Styles) string {
	parts := []string{s.Header.Render(appName)}

	if v.Host != "" {
		parts = append(parts, "host: "+v.Host)
	}
	if v.Root {
		parts = append(parts, s.OK.Render("root"))
	} else {
		parts = append(parts, s.Warn.Render("read-only"))
	}
	if !v.Systemd {
		parts = append(parts, s.Warn.Render("systemd なし"))
	}
	parts = append(parts, githubLabel(v, s))
	if v.HostReq > 0 {
		// ジョブ実行の前提（FR-43）はホストの能力そのものなので、能力判定の
		// バッジと同じ行に出す。件数だけを出し、内訳と対処は Doctor タブが持つ
		// （誘導は状態行が行う。screens.md の共通レイアウト）。
		parts = append(parts, s.Warn.Render(token.IconWarn+" ホスト前提 "+strconv.Itoa(v.HostReq)+" 件"))
	}

	return atom.Join(parts, "  ", width, token.IconEllipsis)
}

// githubLabel は gh の認証状態を返す。
func githubLabel(v CapsView, s token.Styles) string {
	switch {
	case !v.HasToken:
		return s.Warn.Render("gh: 未認証")
	case v.GitHubUser == "":
		// 認証済みだがユーザー名を取得できていない状態（API 応答待ちなど）。
		return "gh: " + s.Muted.Render("認証済み")
	default:
		return "gh: " + v.GitHubUser
	}
}
