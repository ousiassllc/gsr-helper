package ui

import (
	"github.com/ousiassllc/gsr-helper/internal/ui/chrome"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
	"github.com/ousiassllc/gsr-helper/internal/ui/tabset"
)

// 本体以外の領域（ヘッダ・タブ行・状態行・フッタ）の中身は chrome が組み立てる。
// ここに残すのは、親 Model が持つ値を chrome の入力へ写す 1 手だけである。

// pageChrome はタブ番号だけを埋めた空の ChromeMsg を返す。
//
// タブを切り替えた直後に前のタブのモーダル・入力中・フッタが残らないようにする。
func pageChrome(tab int) page.ChromeMsg {
	return page.ChromeMsg{Tab: tab, Modal: false, Input: "", Status: "", Footer: nil}
}

// chromeView は 1 フレーム分の入力を組み立てる。
//
// chrome は molecule 階層なのでドメインの型（appconfig.Caps / runner.Result）を
// 受け取らない。表示に使う値へ落とすのは上位である親 Model の役目であり、その
// 変換をここ 1 箇所に集めている（atomic-design.md の依存の規則）。
func (a App) chromeView() chrome.View {
	return chrome.View{
		Host:        a.opts.Host,
		Root:        a.caps.Root,
		Systemd:     a.caps.Systemd,
		HasToken:    a.caps.GitHubToken,
		Tabs:        tabset.Views(a.tabs, a.active),
		OrphanUnits: len(a.result.OrphanUnits),
		Warnings:    len(a.result.Warnings),
		HostReq:     a.hostReq,
		DoctorKey:   a.doctorKey(),
		Err:         a.err,
		Notice:      a.notice,
		Status:      a.chrome.Status,
		Hints:       a.chrome.Footer,
		Width:       a.width,
		Styles:      a.styles,
	}
}
