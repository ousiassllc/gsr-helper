package page

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/ui/atom"
	"github.com/ousiassllc/gsr-helper/internal/ui/keymap"
	"github.com/ousiassllc/gsr-helper/internal/ui/organism/pane"
)

// helpTitle はヘルプのモーダルの見出し。
const helpTitle = "ヘルプ"

// HelpScope は Set から ? に出すキーのグループを選ぶ関数。
//
// グループの配列ではなく関数で受けるのは、配色やキー定義が差し替わったときに
// Overlay が同じ範囲で組み直せるようにするためである。画面は
// keymap.Set.RunnerListHelp のようなメソッド値を渡す。
type HelpScope func(keymap.Set) [][]key.Binding

// helpModal は ? の全キー一覧のモーダル。pane.Help を tea.Model として包む。
//
// Overlay は中身を tea.Model としてしか知らないため（Modal の doc）、organism の
// 部品にはこの薄い包みを被せる。
type helpModal struct {
	help  pane.Help
	keys  keymap.Set
	scope HelpScope
	// size は最後に受け取った領域。作り直した Help へ、位置を戻す**前に**配り直す。
	size SizeMsg
}

// tea.Model を実装していることをコンパイル時に確かめる。
var _ tea.Model = helpModal{}

// newHelpModal は全キー一覧のモーダルを組み立てる。
func newHelpModal(st StateMsg, scope HelpScope) Modal {
	m := helpModal{
		help:  pane.NewHelp(st.Styles, scope(st.Keys), st.Keys.List),
		keys:  st.Keys,
		scope: scope,
		size:  SizeMsg{W: 0, H: 0},
	}
	return Modal{
		Model: m,
		Title: func(tea.Model) string { return helpTitle },
		Hints: helpHints,
		// esc は常に 1 枚閉じる。ヘルプは入力も編集も持たない表示専用のモーダルで
		// あり、esc に「閉じる」以外の意味を与えない。
		HandlesBack: nil,
	}
}

// Init は何も発行しない。開くタイミングは Overlay が決める。
func (m helpModal) Init() tea.Cmd { return nil }

// Update は共有状態の反映・大きさの設定・スクロールのキーを処理する。
func (m helpModal) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case StateMsg:
		// 配色とキー定義が変わったら同じ範囲で組み直し、スクロール位置は引き継ぐ。
		// 位置を戻すと、背景色の応答が届いただけで読んでいた場所を失う。
		//
		// **大きさを配り直してから位置を戻す。** SetOffset は高さで丸めるため、
		// 高さが未確定（NewHelp の直後は 0）のうちに呼ぶと位置は 0 に潰れ、後から
		// SizeMsg で高さが入っても戻らない。共有状態は 3 秒ごとに届くので、
		// 順序を誤るとヘルプを読んでいる間ずっと先頭へ戻され続ける。
		off := m.help.Offset()
		m.keys = msg.Keys
		m.help = pane.NewHelp(msg.Styles, m.scope(msg.Keys), msg.Keys.List)
		m.help.SetSize(m.size.W, m.size.H)
		m.help.SetOffset(off)
		return m, nil
	case SizeMsg:
		m.size = msg
		m.help.SetSize(msg.W, msg.H)
		return m, nil
	default:
		var cmd tea.Cmd
		m.help, cmd = m.help.Update(msg)
		return m, cmd
	}
}

// View は全キーの一覧を返す。
func (m helpModal) View() tea.View { return tea.NewView(m.help.View()) }

// helpHints はヘルプのフッタに出すキーヒントを返す。
//
// スクロールが必要なときだけ移動キーを出す。収まっているのに「j/k:スクロール」を
// 出すと、押しても何も起きないキーを画面に出すことになる（screens.md の設計原則 2）。
func helpHints(model tea.Model) []atom.Hint {
	m, ok := model.(helpModal)
	if !ok {
		return nil
	}

	hints := make([]atom.Hint, 0, 2)
	if m.help.Scrollable() {
		hints = append(hints, atom.Hint{
			Key:     BindingKey(m.keys.List.Down) + "/" + BindingKey(m.keys.List.Up),
			Desc:    "スクロール",
			Enabled: true,
			Reason:  "",
		})
	}
	return append(hints, atom.Hint{
		Key:     BindingKey(m.keys.Global.Back),
		Desc:    "閉じる",
		Enabled: true,
		Reason:  "",
	})
}
