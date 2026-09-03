// Package keymap はキーとヘルプ文言の定義を集める。
//
// token と並ぶ最下層であり、bubbles/key 以外に依存しない。キー定義が page ごとに
// 散ると、フッタ・ヘルプ・詳細画面の操作リストで説明文が食い違うため 1 箇所に集める。
//
// どの関数も呼び出しごとに新しい値を返す。片方の Binding を無効化しても
// 他の呼び出し元に影響しないようにするためである。
package keymap

import "charm.land/bubbles/v2/key"

// Global はどの画面でも有効なキー。
//
// 入力中（絞り込み・フィルタ・フォーム）とモーダル表示中は Interrupt 以外が
// 効かない。その判断は page の責務であり、ここでは定義のみを持つ。
type Global struct {
	TabSelect key.Binding // タブの直接選択
	TabNext   key.Binding // 次のタブ
	TabPrev   key.Binding // 前のタブ
	Refresh   key.Binding // 手動で再読み込み
	Help      key.Binding // 全キーの一覧
	Quit      key.Binding // 終了
	Interrupt key.Binding // 終了（入力中・モーダル表示中も有効）
	Back      key.Binding // 選択のクリア / 1 つ前の状態へ戻る
}

// NewGlobal はグローバルキーの定義を返す。
func NewGlobal() Global {
	return Global{
		TabSelect: key.NewBinding(
			key.WithKeys("1", "2", "3", "4", "5", "6", "7"),
			key.WithHelp("1〜7", "タブを選択"),
		),
		TabNext: key.NewBinding(
			key.WithKeys("tab"),
			key.WithHelp("tab", "次のタブ"),
		),
		TabPrev: key.NewBinding(
			key.WithKeys("shift+tab"),
			key.WithHelp("shift+tab", "前のタブ"),
		),
		Refresh: key.NewBinding(
			key.WithKeys("r"),
			key.WithHelp("r", "再読み込み"),
		),
		Help: key.NewBinding(
			key.WithKeys("?"),
			key.WithHelp("?", "ヘルプ"),
		),
		Quit: key.NewBinding(
			key.WithKeys("q"),
			key.WithHelp("q", "終了"),
		),
		Interrupt: key.NewBinding(
			key.WithKeys("ctrl+c"),
			key.WithHelp("ctrl+c", "終了"),
		),
		Back: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "戻る"),
		),
	}
}

// Bindings はグローバルキーをヘルプに並べる順で返す。
func (g Global) Bindings() []key.Binding {
	return []key.Binding{
		g.TabSelect, g.TabNext, g.TabPrev, g.Refresh, g.Help, g.Quit, g.Interrupt, g.Back,
	}
}
