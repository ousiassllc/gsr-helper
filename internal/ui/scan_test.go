package ui

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/ui/page"
)

// scanKey は束のどの位置・どの深さにある差し戻しも、遅れて返る差し戻しも取りこぼさない。
//
// **取りこぼしは「閉じ込めが効いた」と区別できない。** press1 は差し戻しが無いとき
// nil を返し、gate_test の 6 つの assertion はすべて nil で満たされる。つまり走査が
// 取りこぼすと、閉じ込めが壊れていてもテストが緑になる（Issue #31 の主題そのもの）。
// 過去に破れた 2 つの当て推量を、破れた形として名前付きで残す。
//
//   - 「先頭が ChromeMsg なら差し戻しは無い」… tea.Batch が 1 本の束を畳むため、
//     実 page もモーダル表示中は tea.Batch(chrome, BubbleKey) と平らに返す
//   - 「すぐ返らない Cmd は差し戻しではない」… 遅れた差し戻しを打ち切って見失う
func TestScanKeyFindsBubbleInEveryShape(t *testing.T) {
	key := press("q")
	chromeCmd := func() tea.Msg {
		return page.ChromeMsg{Tab: 0, Modal: true, Input: "", Status: "", Footer: nil}
	}
	// 絞り込み中の一覧が返すカーソル点滅を模した、遅くて無関係な Cmd。
	slow := func() tea.Msg { time.Sleep(20 * time.Millisecond); return tickMsg{} }
	// 遅れて返る差し戻し。時間で打ち切る走査はこれを見失う。
	slowBubble := func() tea.Msg {
		time.Sleep(20 * time.Millisecond)
		return page.GlobalKeyMsg{Press: key}
	}

	tests := map[string]struct {
		cmd  tea.Cmd
		want bool
	}{
		"最上位の末尾（既定の形）":              {tea.Batch(tea.Batch(chromeCmd, slow), page.BubbleKey(key)), true},
		"ChromeMsg の後ろで平ら（束が畳まれた形）": {tea.Batch(chromeCmd, page.BubbleKey(key)), true},
		"入れ子の中": {tea.Batch(chromeCmd, tea.Batch(slow, page.BubbleKey(key))), true},
		"遅れて返る": {tea.Batch(chromeCmd, slowBubble), true},
		"差し戻し無し（閉じ込め。点滅だけ）": {tea.Batch(chromeCmd, slow), false},
		"ChromeMsg だけ": {chromeCmd, false},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			c, global, ok := scanKey(tt.cmd)
			if ok != tt.want {
				t.Fatalf("差し戻しの検出 = %v, want %v", ok, tt.want)
			}
			if ok && global.Press.String() != "q" {
				t.Errorf("差し戻されたキー = %q, want q", global.Press.String())
			}
			if !c.Modal {
				t.Error("ChromeMsg を取りこぼした")
			}
		})
	}
}
