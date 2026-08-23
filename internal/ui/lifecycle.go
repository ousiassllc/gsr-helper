package ui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/ui/page"
	"github.com/ousiassllc/gsr-helper/internal/ui/tabset"
)

// 起動時に選択されているタブへ前面化を知らせる（Issue #63）。タブの切り替えに伴う
// 通知は keys.go の activate が持つ。ここにあるのは「1 度目」だけである。

// activateInitial は起動時に選択されているタブへ page.ActivateMsg を 1 度だけ配る。
//
// **配る場所が Init ではなく distribute なのは、Init が Model を書き換えられない
// （Cmd だけを返す）ためである。** 配ったことを覚えられないと、共有状態が配られる
// たびに（端末サイズ・背景色・3 秒ごとの再検出）同じタブへ前面化が届き、
// page.ActivateMsg で購読を張る page が周期ごとに 1 本ずつ購読を増やす。
//
// activate（タブの切り替え）と役割を分けてあるのは、切り替えでは離れるタブへの
// page.DeactivateMsg と対になる必要があるのに対し、起動時の 1 度目には対になる
// 相手が居ないためである。往復して戻ってきたときの前面化は activate が配る。
func (a *App) activateInitial() tea.Cmd {
	if a.activated || !tabset.Live(a.tabs, a.active) {
		// 配れないうちは覚えない。有効な page が入るのを待って次の機会に配る。
		return nil
	}
	a.activated = true
	return tabset.Deliver(a.tabs, a.active, page.ActivateMsg{})
}
