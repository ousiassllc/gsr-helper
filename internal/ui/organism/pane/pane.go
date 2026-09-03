// Package pane は行を縦に流してスクロールする領域を提供する。
//
// organism の部品を性質で 2 つに分け、カーソルと選択を持つ対話的な一覧・選択
// （organism.Table / organism.ChoiceList）は organism に、行を縦に流してスクロールする
// 領域（Detail / Help / Log）はこのパッケージに置く。持つ状態も検証の観点も違うためである。
//
// **分かれ目は「表示専用か」でも「bubbles/viewport を使うか」でもない。** Log はフィルタの
// 入力欄（textinput.Model）と入力モードを持つので表示専用ではない。Help は viewport を使わず、
// bubbles/help が組んだ全キー一覧を自前の offset で切り出してスクロールする（viewportKeyMap を
// 共有するのは Detail と Log の 2 つだけである）。それでも 3 つをここに置くのは、持つ状態
// （先頭から何行隠しているか）も検証の観点（期待する行が見えているか）も同じだからである。
// 入力欄の有無や viewport の有無という実装の道具で分けると、スクロールの扱いが 2 つの階層に
// 割れて keymap との対応づけが追えなくなる。
//
// organism とこのパッケージは互いに import しない（どちらの向きの参照も作らない）。
// 両者は独立した部品の集まりであり、必要なものを選んで組み合わせるのは page の役割である。
// import するのは atom / molecule / token / keymap と bubbles / bubbletea / lipgloss に限り、
// page とドメイン層は import しない。
//
// 型名に階層名を重ねない規約に従い、型は Detail / Help / Log と呼ぶ（DetailPane とはしない）。
package pane

import (
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/viewport"

	"github.com/ousiassllc/gsr-helper/internal/ui/keymap"
)

// viewportKeyMap は bubbles/viewport に渡すキー定義を keymap の定義から組み立てる。
//
// viewport.DefaultKeyMap をそのまま使えないのは、既定では半ページ送りが u / d、ページ送りが
// f / b / space、左右スクロールが h / l に割り当てられており、本ツールの u（バージョン更新）/
// d（ドレイン停止）/ space（選択のトグル）/ l（ログを開く）がスクロールに食われるためである。
// 半ページ送りと横スクロールは screens.md のキーマップに無いので割り当てない。
func viewportKeyMap(l keymap.List) viewport.KeyMap {
	return viewport.KeyMap{
		Up:           l.Up,
		Down:         l.Down,
		PageUp:       l.PageUp,
		PageDown:     l.PageDown,
		HalfPageUp:   unbound(),
		HalfPageDown: unbound(),
		Left:         unbound(),
		Right:        unbound(),
	}
}

// unbound はどのキーにも一致しない Binding を返す。
//
// Binding を消さずキーの無い Binding を置くのは、KeyMap の項目を埋め忘れたのか
// 意図して無効にしたのかを区別するためである。
func unbound() key.Binding {
	return key.NewBinding()
}
