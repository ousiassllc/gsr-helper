package dialog_test

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/ousiassllc/gsr-helper/internal/ui/token"
)

// press はキー入力の Msg を作る。文字キーは Text、特殊キーは Code で表す
// （bubbletea v2 の Key.String は Text があればそれを、無ければ keystroke を返す）。
//
// organism / organism/pane の helper_test.go と同じ実装である。テストヘルパーは
// パッケージをまたいで共有できないため、各パッケージに置く。
func press(k string) tea.KeyPressMsg {
	special := map[string]rune{
		"space": tea.KeySpace, "enter": tea.KeyEnter, "esc": tea.KeyEscape, "tab": tea.KeyTab,
		"up": tea.KeyUp, "down": tea.KeyDown, "left": tea.KeyLeft, "right": tea.KeyRight,
	}
	if code, ok := special[k]; ok {
		return tea.KeyPressMsg{Code: code}
	}
	if rest, ok := strings.CutPrefix(k, "ctrl+"); ok {
		return tea.KeyPressMsg{Code: rune(rest[0]), Mod: tea.ModCtrl}
	}
	return tea.KeyPressMsg{Text: k, Code: []rune(k)[0]}
}

// testStyles は色を使わないスタイル。期待文字列に ANSI 列が混ざらないようにする。
func testStyles() token.Styles {
	return token.NewStyles(true, false)
}

// wantNoWideLine はどの行も幅を超えていないことを確かめる。
//
// **ダイアログは幅を超える行を出してはならない。** 超えた行はモーダルの枠の中で
// 折り返して行数が増え、下辺（╰…╯）が領域の外へ押し出される（template.Modal）。
func wantNoWideLine(t *testing.T, view string, width int) {
	t.Helper()

	for i, line := range strings.Split(view, "\n") {
		if w := lipgloss.Width(line); w > width {
			t.Errorf("%d 行目の幅 = %d, want <= %d（%q）", i+1, w, width, line)
		}
	}
}
