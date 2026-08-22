package keymap

import "charm.land/bubbles/v2/key"

// List は一覧（Runners / Jobs / Disk / Doctor）で有効なキー。
type List struct {
	Up       key.Binding // 上へ
	Down     key.Binding // 下へ
	Top      key.Binding // 先頭へ
	Bottom   key.Binding // 末尾へ
	PageDown key.Binding // 次のページ
	PageUp   key.Binding // 前のページ

	Toggle    key.Binding // 選択のトグル
	SelectAll key.Binding // 全選択

	Filter key.Binding // 絞り込みを開始する
	Accept key.Binding // 入力中の確定
	Cancel key.Binding // 入力中の取消

	Enter key.Binding // 詳細を開く
}

// NewList は一覧のキーの定義を返す。
//
// Accept / Cancel は入力中（絞り込み）にのみ有効で、通常時の Enter / Global.Back と
// 同じキーを共有する。入力中はグローバルキーを解釈しないという配送の規則
// （atomic-design.md のキー入力の配送）により、同時に有効になることはない。
//
// 区画（セクション）の移動キーは持たない。区画をまたぐ移動は j / k の端越えに
// 委ねる（atomic-design.md の「末尾で j を押すと次の区画の先頭へ」）。tab を
// 割り当てると Runners タブで Global.TabNext と同時に有効になり、
// 打鍵が別の操作として解釈される。
func NewList() List {
	return List{
		Up: key.NewBinding(
			key.WithKeys("k", "up"),
			key.WithHelp("k/↑", "上へ"),
		),
		Down: key.NewBinding(
			key.WithKeys("j", "down"),
			key.WithHelp("j/↓", "下へ"),
		),
		Top: key.NewBinding(
			key.WithKeys("g"),
			key.WithHelp("g", "先頭へ"),
		),
		Bottom: key.NewBinding(
			key.WithKeys("G"),
			key.WithHelp("G", "末尾へ"),
		),
		PageDown: key.NewBinding(
			key.WithKeys("ctrl+f"),
			key.WithHelp("ctrl+f", "次のページ"),
		),
		PageUp: key.NewBinding(
			key.WithKeys("ctrl+b"),
			key.WithHelp("ctrl+b", "前のページ"),
		),
		Toggle: key.NewBinding(
			key.WithKeys("space"),
			key.WithHelp("space", "選択のトグル"),
		),
		SelectAll: key.NewBinding(
			key.WithKeys("ctrl+a"),
			key.WithHelp("ctrl+a", "全選択"),
		),
		Filter: key.NewBinding(
			key.WithKeys("/"),
			key.WithHelp("/", "絞り込み"),
		),
		Accept: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "確定"),
		),
		Cancel: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "取消"),
		),
		Enter: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "詳細を開く"),
		),
	}
}

// Bindings は一覧のキーをヘルプに並べる順で返す。
func (l List) Bindings() []key.Binding {
	return []key.Binding{
		l.Up, l.Down, l.Top, l.Bottom, l.PageDown, l.PageUp,
		l.Toggle, l.SelectAll,
		l.Filter, l.Accept, l.Cancel,
		l.Enter,
	}
}
