package pane_test

import (
	"strconv"
	"testing"

	"charm.land/bubbles/v2/key"

	"github.com/ousiassllc/gsr-helper/internal/ui/keymap"
	"github.com/ousiassllc/gsr-helper/internal/ui/organism/pane"
)

// スクロール位置の読み書き（Help.Offset / SetOffset、Detail.GotoTop / Offset）を
// 自パッケージで固定する。いずれも本番の呼び出し元（page/helpmodal.go、
// page/runnerdetail/detail.go）から使われる API である（Issue #30）。

// scrollGroups は高さに収まらない量のキーを 1 グループで返す。
func scrollGroups(n int) [][]key.Binding {
	group := make([]key.Binding, 0, n)
	for i := range n {
		k := "k" + strconv.Itoa(i)
		group = append(group, key.NewBinding(key.WithKeys(k), key.WithHelp(k, "説明"+k)))
	}
	return [][]key.Binding{group}
}

// SetOffset は高さで丸め、Offset はその結果を返す。
//
// **高さが決まる前に呼ぶと 0 に潰れる。** 作り直して位置を引き継ぐ側は、大きさを
// 配り直してから戻さなければならない（atomic-design.md の配り直しの義務）。
func TestHelpOffsetRoundsByHeight(t *testing.T) {
	h := pane.NewHelp(testStyles(), scrollGroups(20), keymap.NewList())

	// 高さが未確定のうちは丸め先が 0 になる。
	h.SetOffset(5)
	if got := h.Offset(); got != 0 {
		t.Errorf("高さ未確定での位置 = %d, want 0", got)
	}

	h.SetSize(40, 5)
	if !h.Scrollable() {
		t.Fatal("高さに収まらない量のキーでスクロールできない（前提が崩れている）")
	}

	h.SetOffset(3)
	if got := h.Offset(); got != 3 {
		t.Errorf("位置 = %d, want 3", got)
	}
	h.SetOffset(-1)
	if got := h.Offset(); got != 0 {
		t.Errorf("負の位置 = %d, want 0", got)
	}

	// 上限を超える指定は末尾へ丸める。
	h.SetOffset(9999)
	top := h.Offset()
	if top == 0 {
		t.Fatal("末尾へ丸めた位置が 0 である")
	}

	// 幅を狭めると行数が増えるが、位置は収まる範囲に保たれる。
	h.SetSize(40, 3)
	if got := h.Offset(); got < 0 {
		t.Errorf("高さを縮めた後の位置 = %d, want 0 以上", got)
	}
}

// キー操作でも位置は Offset に反映される（本番と同じ経路）。
func TestHelpScrollKeysMoveOffset(t *testing.T) {
	h := pane.NewHelp(testStyles(), scrollGroups(20), keymap.NewList())
	h.SetSize(40, 5)

	h, _ = h.Update(press("j"))
	h, _ = h.Update(press("j"))
	if got := h.Offset(); got != 2 {
		t.Errorf("j 2 回の後の位置 = %d, want 2", got)
	}

	h, _ = h.Update(press("k"))
	if got := h.Offset(); got != 1 {
		t.Errorf("k の後の位置 = %d, want 1", got)
	}
}

// Detail は位置を読み書きでき、GotoTop で先頭へ戻る。
//
// 対象そのものを差し替える側（別の runner の詳細を開く）が呼ぶ API である。
func TestDetailGotoTop(t *testing.T) {
	lines := make([]string, 0, 20)
	for i := range 20 {
		lines = append(lines, "行"+strconv.Itoa(i))
	}

	d := pane.NewDetail()
	d.SetContent(lines)
	d.SetSize(40, 3)

	d, _ = d.Update(press("down"))
	d, _ = d.Update(press("down"))
	if got := d.Offset(); got == 0 {
		t.Fatal("スクロールしても位置が 0 のままである（前提が崩れている）")
	}

	d.GotoTop()
	if got := d.Offset(); got != 0 {
		t.Errorf("GotoTop の後の位置 = %d, want 0", got)
	}
}
