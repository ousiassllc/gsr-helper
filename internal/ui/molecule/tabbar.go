package molecule

import (
	"github.com/ousiassllc/gsr-helper/internal/ui/atom"
	"github.com/ousiassllc/gsr-helper/internal/ui/token"
)

// TabView はタブ 1 枚の表示用の構造体。
type TabView struct {
	Key     string // タブを直接選ぶキー（"1"〜"7"）
	Title   string // タブ名
	Active  bool   // 選択中か
	Enabled bool   // 選べるか
}

// TabBar はタブ行を返す。
//
// 選択中・選択可・選択不可は色だけで区別しない（screens.md の設計原則 4）。
// 選択中は先頭にカーソル記号を付け、選択不可はキーの括弧を丸括弧に変える。
// 色を使えない端末でも 3 状態が読み分けられるようにするためである。
func TabBar(tabs []TabView, width int, s token.Styles) string {
	parts := make([]string, 0, len(tabs))
	for _, t := range tabs {
		parts = append(parts, tabLabel(t, s))
	}
	// 区切りは空文字にする。tabLabel が先頭に 1 セル（カーソル記号かその空白）を
	// 持つため、これがタブ間の空白を兼ねる。ここで空白を挟むと 2 空白になり、
	// screens.md の共通レイアウト（`[1]Runners [2]Jobs …`）と桁が合わない。
	return atom.Join(parts, "", width, token.IconEllipsis)
}

// tabLabel はタブ 1 枚の表示を返す。
//
// 先頭の 1 セルは選択中のカーソル記号の場所で、非選択のタブは同幅の空白を置く。
// 選択タブが変わっても桁が動かないようにするためである。タブ間の空白もこの 1 セルが
// 兼ねる（TabBar は区切りを挟まない）。
func tabLabel(t TabView, s token.Styles) string {
	switch {
	case !t.Enabled:
		return " (" + t.Key + ")" + s.TabDisabled.Render(t.Title)
	case t.Active:
		return s.Cursor.Render(token.IconCursor) + "[" + t.Key + "]" + s.TabActive.Render(t.Title)
	default:
		return " [" + t.Key + "]" + s.TabInactive.Render(t.Title)
	}
}
