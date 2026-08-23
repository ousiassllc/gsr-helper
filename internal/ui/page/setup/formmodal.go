package setup

import (
	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/ui/atom"
	"github.com/ousiassllc/gsr-helper/internal/ui/organism/dialog"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
	"github.com/ousiassllc/gsr-helper/internal/ui/token"
)

// formKind は追加フォームのモーダルの種類。
const formKind page.ModalKind = "setupform"

// formOpenMsg は追加フォームを開く指示。
type formOpenMsg struct {
	kind   formKindOf
	values *formValues
	st     page.StateMsg
}

// formModal は追加フォーム。dialog.Form を包むだけで判断は持たない。
type formModal struct {
	tab  int
	form dialog.Form
	// color は huh のテーマを組み直すときに使う。共有状態から受け取る。
	color bool
}

// tea.Model を実装していることをコンパイル時に確かめる。
var _ tea.Model = formModal{}

// newFormModal は追加フォームのモーダルを組み立てる。
func newFormModal(st page.StateMsg) page.Modal {
	color := st.Color
	return page.Modal{
		Model: formModal{tab: 0, form: dialog.NewForm(st.Styles, color), color: color},
		Title: formTitle,
		Hints: formHints,
		// esc は dialog.Form が受ける。入力済みなら破棄の確認を出すため、
		// Overlay に閉じさせてはならない（atomic-design.md「Form と huh」）。
		HandlesBack: func(tea.Model) bool { return true },
	}
}

// Init は何も発行しない。
func (m formModal) Init() tea.Cmd { return nil }

// Update はフォームへ Msg を配り、完了・中断を page へ差し戻す。
func (m formModal) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case page.AttachMsg:
		m.tab = msg.Tab
		return m, nil
	case formOpenMsg:
		return m.open(msg)
	case dialog.FormDoneMsg, dialog.FormAbortedMsg, dialog.FormDiscardMsg:
		res := page.ResultMsg{Kind: formKind, Msg: msg}
		return m, page.Do(m.tab, func() tea.Msg { return res })
	case page.StateMsg:
		m.color = msg.Color
		m.form.Restyle(msg.Styles, m.color)
		return m, nil
	case page.SizeMsg:
		m.form.SetSize(msg.W, msg.H)
		return m, nil
	default:
		var cmd tea.Cmd
		m.form, cmd = m.form.Update(msg)
		return m, page.WrapModal(m.tab, formKind, cmd)
	}
}

// open は指定された種類のフォームを組み立てて表示する。
func (m formModal) open(msg formOpenMsg) (tea.Model, tea.Cmd) {
	m.color = msg.st.Color
	m.form.SetTitle(msg.kind.title())
	cmd := m.form.SetForm(msg.values.build(token.HuhTheme(msg.st.Styles, m.color)))
	return m, page.WrapModal(m.tab, formKind, cmd)
}

// View はフォームの中身を返す。
func (m formModal) View() tea.View { return tea.NewView(m.form.View()) }

// formTitle はモーダルの見出しを返す。
func formTitle(model tea.Model) string {
	m, ok := model.(formModal)
	if !ok {
		return ""
	}
	return m.form.Title()
}

// formHints はモーダルのフッタを返す。
func formHints(model tea.Model) []atom.Hint {
	m, ok := model.(formModal)
	if !ok {
		return nil
	}
	return m.form.Hints()
}
