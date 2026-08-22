package ui

import (
	"strconv"
	"strings"

	"github.com/ousiassllc/gsr-helper/internal/ui/atom"
	"github.com/ousiassllc/gsr-helper/internal/ui/molecule"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
	"github.com/ousiassllc/gsr-helper/internal/ui/token"
)

// 本体以外の領域（ヘッダ・タブ行・状態行・フッタ）の組み立てを集める。
// 領域の配分は template.Frame が行い、ここでは中身の文字列だけを作る。

// pageChrome はタブ番号だけを埋めた空の ChromeMsg を返す。
//
// タブを切り替えた直後に前のタブのモーダル・入力中・フッタが残らないようにする。
func pageChrome(tab int) page.ChromeMsg {
	return page.ChromeMsg{Tab: tab, Modal: false, Input: "", Status: "", Footer: nil}
}

// header は能力判定の結果を出すヘッダ行を返す。
//
// gh の認証ユーザー名は載せない。Caps はトークンそのものを持たない設計であり、
// ユーザー名の取得は GitHub API を使う機能の担当である（molecule 側は認証済みで
// ユーザー名が無い状態を「認証済み」と描く）。
func (a App) header() string {
	return molecule.CapsBar(molecule.CapsView{
		Host:       a.opts.Host,
		Root:       a.caps.Root,
		Systemd:    a.caps.Systemd,
		GitHubUser: "",
		HasToken:   a.caps.GitHubToken,
	}, a.width, a.styles)
}

// tabBar はタブ行を返す。
func (a App) tabBar() string {
	views := make([]molecule.TabView, 0, len(a.tabs))
	for i := range a.tabs {
		views = append(views, molecule.TabView{
			Key:     a.tabs[i].Key,
			Title:   a.tabs[i].Title,
			Active:  i == a.active,
			Enabled: a.tabs[i].Enabled,
		})
	}
	return molecule.TabBar(views, a.width, a.styles)
}

// status は状態行を返す。
//
// 左側は親が持つ検出結果から作る（孤児ユニット件数・警告件数・直近のエラー）。
// 右側は page が報告した文（選択件数・入力中）を置く。同じ検出結果からの件数を
// タブごとに作らせないため、この分担にしている。
func (a App) status() string {
	return atom.Justify(a.counts(), a.right(), a.width)
}

// right は状態行の右側を返す。
//
// 無効なタブの理由は page の報告より優先する。番号キーを押した打鍵では page へ
// キーが渡らず ChromeMsg も更新されないため、理由を出せるのはここだけである。
func (a App) right() string {
	if a.notice != "" {
		return a.styles.Muted.Render(a.notice)
	}
	return a.chrome.Status
}

// counts は検出結果から状態行の左側を組み立てる。
func (a App) counts() string {
	var parts []string
	if n := len(a.result.OrphanUnits); n > 0 {
		parts = append(parts, a.styles.Warn.Render(
			token.IconWarn+" 孤児ユニット "+strconv.Itoa(n)+" 件"))
	}
	if n := len(a.result.Warnings); n > 0 {
		parts = append(parts, a.styles.Warn.Render("警告 "+strconv.Itoa(n)+" 件"))
	}
	if a.err != nil {
		// エラーで画面遷移を巻き戻さず、状態行に出すだけにする
		// （architecture/overview.md のエラーハンドリング）。
		parts = append(parts, a.styles.Fail.Render(token.IconFailed+" "+a.err.Error()))
	}
	return strings.Join(parts, " / ")
}

// footer はフッタ 2 行を返す。
//
// キーヒントは page が可否と理由込みで報告したものを使う。?:ヘルプ は
// molecule.KeyBar が必ず付けるため、ここでは足さない。
func (a App) footer() string {
	return molecule.KeyBar(a.chrome.Footer, a.width, a.styles)
}
