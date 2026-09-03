package page

import (
	"charm.land/bubbles/v2/key"
)

// キー定義と表示のあいだの小さな橋渡しを置く。操作の識別と可否の判定は
// page/action にある（ディレクトリの行数の上限に収めるための分割。
// atomic-design.md の「ディレクトリの行数」）。

// ReasonUnsupported はこの版で実装していない操作と無効なタブに共通の理由。
//
// 無効なタブ（ui の tab.Reason）と未対応の操作（action.Allow）が同じ文言を読むのは、
// 利用者にとって両者が同じ意味（この版ではまだ使えない）だからである。同じ意味の
// 文言を 2 箇所に持つと片方だけが直り、同じ状況の説明が画面によって食い違う。
const ReasonUnsupported = "この版では未対応です"

// BindingKey は Binding が受け付ける実際のキー文字列を返す。
//
// ヘルプの表記（Help().Key）ではなく Keys() の先頭を使うのは、organism.ChoiceList が
// 押されたキー（tea.KeyPressMsg.String()）と Choice.Key を突き合わせるためである。
// page/<tab> もフッタのキー表記をこの関数で作り、詳細画面と表記が食い違わないように
// する（keymap の Help().Key は "j/↓" のように複数キーをまとめた表記になる）。
func BindingKey(b key.Binding) string {
	if ks := b.Keys(); len(ks) > 0 {
		return ks[0]
	}
	return b.Help().Key
}
