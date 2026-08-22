package keymap

import "charm.land/bubbles/v2/key"

// Confirm は破壊的操作の確認ダイアログのキー（screens.md の確認ダイアログ）。
//
// **ダイアログ側（organism/dialog.Confirm）に定義を置かない。** キーの定義は
// ui/keymap に集約するという規則（components/overview.md の依存の規則）に従う。
// 部品が自前でキーを組み立てると、同時に有効なキーの重複検査（Set.Contexts）を
// 一度も通らないまま画面に出て、フッタとヘルプの表記も定義から離れる。
type Confirm struct {
	Yes key.Binding // y: 実行
	No  key.Binding // n / enter / esc: キャンセル（既定）
}

// NewConfirm は確認ダイアログのキー定義を返す。
//
// **キャンセル側は 1 つの Binding にまとめる。** n / enter / esc はどれも同じ
// 「キャンセル」であり、別々の Binding にするとフッタに同じ結果のキーが 3 つ並ぶ。
// フッタ 1 行目は幅 80 に収める必要があり（molecule.KeyBar）、他の画面のヒントを
// 押し出す。3 つとも効くことは Help().Key ではなく Keys() が持つ。
//
// **enter をキャンセル側に割り当てる。** 一覧で enter を押して詳細を開き、詳細で
// enter を押して操作を選ぶという連続操作の勢いのまま、確認の enter で破壊的操作が
// 走ることを防ぐためである（screens.md の確認ダイアログ。既定はキャンセル）。
// 実行は y だけで、確認を素通りできるキーを他に作らない。
//
// **esc も Global.Back を借りずにここへ含める。** 確認ダイアログは esc を自分で
// キャンセルとして解釈する（page.Modal.HandlesBack に真を返させる）ので、借りると
// 「戻る」という別の説明文が付いて回り、フッタの表記が実際の動作とずれる。
func NewConfirm() Confirm {
	return Confirm{
		Yes: key.NewBinding(
			key.WithKeys("y"),
			key.WithHelp("y", "実行"),
		),
		No: key.NewBinding(
			key.WithKeys("n", "enter", "esc"),
			key.WithHelp("n", "キャンセル"),
		),
	}
}

// Bindings は確認ダイアログのキーを定義順で返す。
func (c Confirm) Bindings() []key.Binding {
	return []key.Binding{c.Yes, c.No}
}
