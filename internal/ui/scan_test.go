package ui

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/ui/page"
)

// scanProbe は走査の対象にならない Msg。点滅など「差し戻しでも ChromeMsg でもない
// もの」の代役に使う。親が意味を持つ Msg（tickMsg など）を借りると、走査が Update へ
// 渡さないことに気づきにくい。
type scanProbe struct{}

// scanKey は束のどの位置・どの深さにある差し戻しも、遅れて返る差し戻しも取りこぼさない。
//
// **取りこぼしは「閉じ込めが効いた」と区別できない。** press1 は差し戻しが無いとき
// nil を返し、gate_test の 6 つの assertion はすべて nil で満たされる。走査が
// 取りこぼすと、閉じ込めが壊れていてもテストが緑になる（Issue #31 の主題）。過去に
// 破れた 2 つの当て推量を、破れた形として残す。
//
//   - 「先頭が ChromeMsg なら差し戻しは無い」… 束が畳まれると通常の打鍵でも
//     tea.Batch(chrome, BubbleKey) と平らになり、閉じ込めと同じ形になる
//   - 「すぐ返らない Cmd は差し戻しではない」… 遅れた差し戻しを打ち切って見失う
//
// **slowBubble の遅延は撤去したタイムアウト（100ms）と点滅（約 500ms）より大きい。**
// 20ms にしていた頃は、撤去した実装を戻しても 6 ケースすべてが緑のままで、この退行の
// 回帰ガードになっていなかった。
func TestScanKeyFindsBubbleInEveryShape(t *testing.T) {
	key := press("q")
	chromeCmd := func() tea.Msg {
		return page.ChromeMsg{Tab: 0, Modal: true, Input: "", Status: "", Footer: nil}
	}
	// 絞り込み中の一覧が返すカーソル点滅を模した、遅くて走査の対象外の Cmd。
	slow := func() tea.Msg { time.Sleep(20 * time.Millisecond); return scanProbe{} }
	// 遅れて返る差し戻し。時間で打ち切る走査はこれを見失う。
	slowBubble := func() tea.Msg {
		time.Sleep(600 * time.Millisecond)
		return page.GlobalKeyMsg{Press: key}
	}

	tests := map[string]struct {
		cmd  tea.Cmd
		want bool
	}{
		"最上位の末尾（既定の形）":              {tea.Batch(tea.Batch(chromeCmd, slow), page.BubbleKey(key)), true},
		"ChromeMsg の後ろで平ら（束が畳まれた形）": {tea.Batch(chromeCmd, page.BubbleKey(key)), true},
		"ChromeMsg より前":             {tea.Batch(page.BubbleKey(key), chromeCmd), true},
		"入れ子の中":                     {tea.Batch(chromeCmd, tea.Batch(slow, page.BubbleKey(key))), true},
		"遅れて返る":                     {tea.Batch(chromeCmd, slowBubble), true},
		"差し戻し無し（閉じ込め。点滅だけ）":         {tea.Batch(chromeCmd, slow), false},
		"ChromeMsg だけ":              {chromeCmd, false},
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
