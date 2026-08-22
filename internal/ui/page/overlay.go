package page

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/appconfig"
	"github.com/ousiassllc/gsr-helper/internal/runner"
	"github.com/ousiassllc/gsr-helper/internal/ui/atom"
	"github.com/ousiassllc/gsr-helper/internal/ui/keymap"
	"github.com/ousiassllc/gsr-helper/internal/ui/organism/pane"
	"github.com/ousiassllc/gsr-helper/internal/ui/template"
	"github.com/ousiassllc/gsr-helper/internal/ui/token"
)

// helpTitle はヘルプのモーダルの見出し。
const helpTitle = "ヘルプ"

// modalKind は重ねられるモーダルの種類。
//
// 種類ごとに部品を 1 つだけ持つ（同じ種類を 2 枚重ねない）ため、重なりは
// 種類の並びだけで表せる。
type modalKind int

const (
	modalDetail modalKind = iota // runner の詳細画面
	modalHelp                    // ? の全キー一覧
)

// Overlay はモーダルの重なりを管理する。
//
// キーは最上位の 1 枚にのみ渡し、背後の page には届けない。確認中に打った x が
// 背後の一覧で別の停止操作として解釈されることを構造的に防ぐためである
// （screens.md のモーダル表示中）。
//
// **コピーは重なりの実体を共有する。** stack のスライスは写しても同じ配列を指すため、
// page は直前の Update が返した 1 つの値だけを持つこと（organism.Table と同じ約束）。
type Overlay struct {
	keys   keymap.Set
	styles token.Styles
	dark   bool

	stack  []modalKind // 重なり順。末尾が最上位
	detail RunnerDetail
	help   pane.Help

	width  int
	height int
}

// NewOverlay はモーダルの重なりを組み立てる。
func NewOverlay(keys keymap.Set, s token.Styles, dark bool) Overlay {
	return Overlay{
		keys:   keys,
		styles: s,
		dark:   dark,
		stack:  nil,
		detail: NewRunnerDetail(keys, s),
		help:   pane.NewHelp(s, keys.FullHelp()),
		width:  0,
		height: 0,
	}
}

// OpenDetail は runner の詳細画面を開く。
func (o *Overlay) OpenDetail(r runner.Runner, caps appconfig.Caps) {
	o.detail.Open(r, caps)
	o.push(modalDetail)
}

// OpenHelp は全キー一覧を開く。
func (o *Overlay) OpenHelp() {
	o.push(modalHelp)
}

// Close は最上位のモーダルを 1 枚だけ閉じる。
//
// 1 枚ずつ閉じるのは、詳細画面からヘルプを開いた後の esc で詳細まで消えると、
// 利用者が「どこへ戻ったか」を追えなくなるためである。
func (o *Overlay) Close() {
	if len(o.stack) == 0 {
		return
	}
	o.stack = o.stack[:len(o.stack)-1]
}

// Active はモーダルを 1 枚以上開いているかを返す。ChromeMsg.Modal に載せる値である。
func (o Overlay) Active() bool { return len(o.stack) > 0 }

// SetState は共有状態のスナップショットを反映する。
//
// 背景の明暗が変わったときは配色を持つ部品を作り直す。開いている詳細は対象を
// 引き継いで開き直すため操作リストのカーソルは先頭へ戻るが、背景色の応答は起動直後に
// 1 度届くだけなので操作を妨げない（安全側に倒す）。
func (o *Overlay) SetState(st StateMsg) {
	if st.Dark != o.dark {
		o.restyle(st)
	}
	o.SetSize(st.BodyW, st.BodyH)
}

// restyle は配色とキー定義を差し替えて部品を作り直す。
func (o *Overlay) restyle(st StateMsg) {
	target, caps := o.detail.target, o.detail.caps
	o.keys, o.styles, o.dark = st.Keys, st.Styles, st.Dark
	o.detail = NewRunnerDetail(st.Keys, st.Styles)
	o.help = pane.NewHelp(st.Styles, st.Keys.FullHelp())
	o.detail.Open(target, caps)
}

// SetSize はモーダルを置ける領域（本体の領域）を設定する。
func (o *Overlay) SetSize(w, h int) {
	o.width, o.height = w, h
	padW, padH := template.ModalPadding()
	o.detail.SetSize(max(w-padW, 1), max(h-padH, 1))
	o.help.SetSize(max(w-padW, 1), max(h-padH, 1))
}

// Update はキーを最上位のモーダルにのみ渡す。
//
// esc はここで受けて 1 枚閉じる。最上位が受け取る前に閉じる判断をするのは、
// 「戻る」の意味をモーダルの種類ごとに実装させないためである。
func (o Overlay) Update(msg tea.Msg) (Overlay, tea.Cmd) {
	top, ok := o.top()
	if !ok {
		return o, nil
	}

	if press, isKey := msg.(tea.KeyPressMsg); isKey && key.Matches(press, o.keys.Global.Back) {
		o.Close()
		return o, nil
	}
	if top != modalDetail {
		// pane.Help はスクロールのキーを持たない（bubbles/help が全キーを 1 画面に描く）。
		// 最上位がヘルプの間はキーを捨てる。背後の詳細へ流さないためである。
		return o, nil
	}

	var cmd tea.Cmd
	o.detail, cmd = o.detail.Update(msg)
	return o, cmd
}

// View は最上位のモーダルを枠に入れて返す。開いていなければ空文字を返す。
func (o Overlay) View() string {
	top, ok := o.top()
	if !ok {
		return ""
	}

	in := template.ModalInput{
		Title:  o.styles.Header.Render(helpTitle),
		Body:   o.help.View(),
		Width:  o.width,
		Height: o.height,
	}
	if top == modalDetail {
		in.Title = o.styles.Header.Render(o.detail.Title())
		in.Body = o.detail.View()
	}
	return template.Modal(in)
}

// Hints は最上位のモーダルのキーヒントを返す。開いていなければ nil を返す。
func (o Overlay) Hints() []atom.Hint {
	top, ok := o.top()
	switch {
	case !ok:
		return nil
	case top == modalDetail:
		return o.detail.Hints()
	default:
		return []atom.Hint{
			{Key: BindingKey(o.keys.Global.Back), Desc: "閉じる", Enabled: true, Reason: ""},
		}
	}
}

// top は最上位のモーダルを返す。
func (o Overlay) top() (modalKind, bool) {
	if len(o.stack) == 0 {
		return modalDetail, false
	}
	return o.stack[len(o.stack)-1], true
}

// push はモーダルを重ねる。同じ種類が既にあるときは最上位へ動かす。
//
// 種類ごとに部品を 1 つしか持たないため、同じ種類を 2 枚積むと閉じても中身が
// 変わらない「閉じられないモーダル」に見える。
func (o *Overlay) push(kind modalKind) {
	kept := make([]modalKind, 0, len(o.stack)+1)
	for _, k := range o.stack {
		if k != kind {
			kept = append(kept, k)
		}
	}
	kept = append(kept, kind)
	o.stack = kept
}
