package table

import (
	"charm.land/bubbles/v2/key"
	btable "charm.land/bubbles/v2/table"

	"github.com/ousiassllc/gsr-helper/internal/ui/keymap"
)

// tableKeyMap は bubbles/table に渡すキー定義を keymap の定義から組み立てる。
//
// btable.DefaultKeyMap をそのまま使えないのは、既定では半ページ送りが u / d、ページ送りが
// f / b / space に割り当てられており、本ツールの u（バージョン更新）/ d（ドレイン停止）/
// space（選択のトグル）が一覧のスクロールに食われるためである。半ページ送りは
// screens.md のキーマップに無いので割り当てない。
func tableKeyMap(l keymap.List) btable.KeyMap {
	return btable.KeyMap{
		LineUp:       l.Up,
		LineDown:     l.Down,
		PageUp:       l.PageUp,
		PageDown:     l.PageDown,
		HalfPageUp:   unbound(),
		HalfPageDown: unbound(),
		GotoTop:      l.Top,
		GotoBottom:   l.Bottom,
	}
}

// unbound はどのキーにも一致しない Binding を返す。
//
// Binding を消さずキーの無い Binding を置くのは、KeyMap の項目を埋め忘れたのか
// 意図して無効にしたのかを区別するためである。
func unbound() key.Binding {
	return key.NewBinding()
}
