package config

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/config/apply"

	"github.com/ousiassllc/gsr-helper/internal/config/edit"
	"github.com/ousiassllc/gsr-helper/internal/ui/atom"
	"github.com/ousiassllc/gsr-helper/internal/ui/keymap"
	"github.com/ousiassllc/gsr-helper/internal/ui/organism"
	"github.com/ousiassllc/gsr-helper/internal/ui/organism/dialog"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
	"github.com/ousiassllc/gsr-helper/internal/ui/token"
)

// モーダルの種類。
const (
	formKind  page.ModalKind = "configform"
	diffKind  page.ModalKind = "configdiff"
	applyKind page.ModalKind = "configapply"
)

// formOpenMsg はフォームを開く指示。
type formOpenMsg struct {
	title  string
	values *edit.Values
	st     page.StateMsg
}

// formModal は設定編集のフォーム。dialog.Form を包むだけで判断は持たない。
type formModal struct {
	tab   int
	form  dialog.Form
	color bool
}

var _ tea.Model = formModal{}

// newFormModal はフォームのモーダルを組み立てる。
func newFormModal(st page.StateMsg) page.Modal {
	color := st.Color
	return page.Modal{
		Model: formModal{tab: 0, form: dialog.NewForm(st.Styles, color), color: color},
		Title: formTitle,
		Hints: formHints,
		// esc は dialog.Form が受ける。Overlay に閉じさせてはならない。
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
		m.color = msg.st.Color
		m.form.SetTitle(msg.title)
		cmd := m.form.SetForm(buildForm(msg.values, token.HuhTheme(msg.st.Styles, m.color)))
		return m, page.WrapModal(m.tab, formKind, cmd)
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

// diffOpenMsg は差分の承認を開く指示。
type diffOpenMsg struct {
	input dialog.DiffApprovalInput
}

// diffModal は差分プレビューと承認（FR-37 / FR-38）。
type diffModal struct {
	tab int
	dlg dialog.DiffApproval
}

var _ tea.Model = diffModal{}

// newDiffModal は差分の承認のモーダルを組み立てる。
func newDiffModal(st page.StateMsg) page.Modal {
	return page.Modal{
		Model:       diffModal{tab: 0, dlg: dialog.NewDiffApproval(st.Keys, st.Styles)},
		Title:       diffTitle,
		Hints:       diffHints,
		HandlesBack: nil,
	}
}

// Init は何も発行しない。
func (m diffModal) Init() tea.Cmd { return nil }

// Update は決定を page へ差し戻す。
func (m diffModal) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case page.AttachMsg:
		m.tab = msg.Tab
		return m, nil
	case diffOpenMsg:
		m.dlg.SetInput(msg.input)
		return m, nil
	case dialog.DecidedMsg:
		res := page.ResultMsg{Kind: diffKind, Msg: msg}
		return m, page.Do(m.tab, func() tea.Msg { return res })
	case page.StateMsg:
		m.dlg.Restyle(msg.Keys, msg.Styles)
		return m, nil
	case page.SizeMsg:
		m.dlg.SetSize(msg.W, msg.H)
		return m, nil
	default:
		var cmd tea.Cmd
		m.dlg, cmd = m.dlg.Update(msg)
		return m, page.WrapModal(m.tab, diffKind, cmd)
	}
}

// View は差分を描く。
func (m diffModal) View() tea.View { return tea.NewView(m.dlg.View()) }

// diffTitle はモーダルの見出しを返す。
func diffTitle(model tea.Model) string {
	m, ok := model.(diffModal)
	if !ok {
		return ""
	}
	return m.dlg.Title()
}

// diffHints はモーダルのフッタを返す。
func diffHints(model tea.Model) []atom.Hint {
	m, ok := model.(diffModal)
	if !ok {
		return nil
	}
	return m.dlg.Hints()
}

// applyOpenMsg は反映方法の選択を開く指示。
type applyOpenMsg struct {
	items []organism.Choice
}

// applyModal は反映方法の選択（FR-39）。選択肢を並べる UI は
// organism.ChoiceList に統一されているため、包むだけで並び順も既定も持たない。
type applyModal struct {
	tab  int
	list organism.ChoiceList
	keys keymap.Set
}

var _ tea.Model = applyModal{}

// newApplyModal は反映方法の選択のモーダルを組み立てる。
func newApplyModal(st page.StateMsg) page.Modal {
	return page.Modal{
		Model: applyModal{
			tab: 0, list: organism.NewChoiceList(st.Keys.List, st.Styles), keys: st.Keys,
		},
		Title: func(tea.Model) string { return titleApply },
		Hints: applyHints,
		// esc は自分で解釈する。Overlay に閉じさせると apply.Run を通らず、
		// drop-in に要る daemon-reload まで飛んでしまう（Update の doc）。
		HandlesBack: func(tea.Model) bool { return true },
	}
}

// Init は何も発行しない。
func (m applyModal) Init() tea.Cmd { return nil }

// Update は選択を page へ差し戻す。
func (m applyModal) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case page.AttachMsg:
		m.tab = msg.Tab
		return m, nil
	case applyOpenMsg:
		m.list.SetItems(msg.items, organism.ResetCursor)
		return m, nil
	case organism.ChosenMsg:
		return m, m.chose(msg.ID)
	case tea.KeyPressMsg:
		// **esc は「反映しない」を選んだことにする。** フッタにそう書いてあるうえ、
		// 単に閉じると apply.Run を通らない。apply.None は「今すぐ再起動はしない」
		// であって「daemon-reload もしない」ではないので、閉じるだけでは
		// drop-in を置いた systemd が新しい内容を読まないままになる。
		if key.Matches(msg, m.keys.Global.Back) {
			return m, m.chose(apply.None.Label())
		}

		var cmd tea.Cmd
		m.list, cmd = m.list.Update(msg)

		return m, page.WrapModal(m.tab, applyKind, cmd)
	case page.StateMsg:
		m.keys = msg.Keys
		m.list.Restyle(msg.Keys.List, msg.Styles)

		return m, nil
	case page.SizeMsg:
		m.list.SetWidth(msg.W)
		return m, nil
	default:
		var cmd tea.Cmd
		m.list, cmd = m.list.Update(msg)
		return m, page.WrapModal(m.tab, applyKind, cmd)
	}
}

// chose は選ばれた反映方法を page へ差し戻す Cmd を返す。
func (m applyModal) chose(id string) tea.Cmd {
	res := page.ResultMsg{Kind: applyKind, Msg: organism.ChosenMsg{ID: id, Key: ""}}
	return page.Do(m.tab, func() tea.Msg { return res })
}

// View は選択肢を描く。
func (m applyModal) View() tea.View { return tea.NewView(m.list.View()) }

// applyHints は反映方法の選択のフッタを返す。esc は「反映しない」を選ぶ。
func applyHints(model tea.Model) []atom.Hint {
	m, ok := model.(applyModal)
	if !ok {
		return nil
	}

	return []atom.Hint{
		{Key: page.BindingKey(m.keys.List.Enter), Desc: "この方法で反映", Enabled: true, Reason: ""},
		{Key: page.BindingKey(m.keys.Global.Back), Desc: "反映しない", Enabled: true, Reason: ""},
	}
}
