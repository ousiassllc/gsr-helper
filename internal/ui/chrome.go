package ui

import (
	"github.com/ousiassllc/gsr-helper/internal/ui/chrome"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
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
func (a App) chromeView() chrome.View {
	return chrome.View{
		Host:   a.opts.Host,
		Caps:   a.caps,
		Tabs:   a.tabs,
		Active: a.active,
		Result: a.result,
		Err:    a.err,
		Notice: a.notice,
		Status: a.chrome.Status,
		Hints:  a.chrome.Footer,
		Width:  a.width,
		Styles: a.styles,
	}
}
