package keymap

import "charm.land/bubbles/v2/key"

// DiskKeys は Disk タブ固有のキー（screens.md の Disk タブ）。
//
// 対象の選択（space）と再集計（r）は持たない。前者は一覧共通の List.Toggle、後者は
// どの画面でも効く Global.Refresh そのものであり、ここに同じキーを重ねて定義すると
// 同時に有効な Binding が 2 つになる。1 打鍵で 2 つの操作が走る経路を作らないため、
// **既存の定義を使い回す**（Set.Contexts の doc）。
type DiskKeys struct {
	Clean key.Binding // 選択した対象のドライランへ進む
}

// NewDiskKeys は Disk タブのキーの定義を返す。
//
// c を選んだのは screens.md のキーマップに従ったためである。小文字なのは、この操作が
// 削除そのものではなくドライラン（確認ダイアログ）を開くだけで、安全側だからである
// （設計原則 3）。実際の削除は確認ダイアログの y が起点になる。
func NewDiskKeys() DiskKeys {
	return DiskKeys{
		Clean: key.NewBinding(
			key.WithKeys("c"),
			key.WithHelp("c", "クリーンアップ"),
		),
	}
}

// Bindings は Disk タブのキーをヘルプに並べる順で返す。
func (d DiskKeys) Bindings() []key.Binding {
	return []key.Binding{d.Clean}
}

// ConfirmKeys は破壊的操作の確認ダイアログで有効なキー（screens.md の確認ダイアログ）。
//
// esc と enter もキャンセルに割り当てるが、ここでは定義しない。前者は Global.Back、
// 後者は List.Accept であり、同じキーを重ねて定義すると同時に有効な Binding が
// 2 つになる。確認ダイアログ（organism/dialog.Confirm）はこの 2 つと No を
// 同じ「キャンセル」として扱う。
//
// **enter をキャンセル側に置くのは仕様である。** 連続操作の勢いで破壊的操作が
// 実行されることを防ぐ（設計原則 5「Enter の連打では進まない」）。
type ConfirmKeys struct {
	Yes key.Binding // 実行する
	No  key.Binding // キャンセル（既定）
}

// NewConfirmKeys は確認ダイアログのキーの定義を返す。
func NewConfirmKeys() ConfirmKeys {
	return ConfirmKeys{
		Yes: key.NewBinding(
			key.WithKeys("y"),
			key.WithHelp("y", "実行"),
		),
		No: key.NewBinding(
			key.WithKeys("n"),
			key.WithHelp("n", "キャンセル"),
		),
	}
}

// Bindings は確認ダイアログのキーをヘルプに並べる順で返す。
//
// 実行を先に置くのは、ヘルプの並びを画面の問い（[y/N]）と揃えるためである。
func (c ConfirmKeys) Bindings() []key.Binding {
	return []key.Binding{c.Yes, c.No}
}
