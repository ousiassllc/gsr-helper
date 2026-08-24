package setupmodal

import (
	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/ui/atom"
	"github.com/ousiassllc/gsr-helper/internal/ui/organism/dialog"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
)

// モーダルの種類。タブ名を接頭辞に付けるのは、Overlay が種類の重複で panic する
// ためである。
const (
	// ConfirmKind は実行前プレビュー。
	ConfirmKind page.ModalKind = "setupconfirm"
	// DiscardKind はフォームの入力を破棄してよいかの確認。
	//
	// 実行前プレビューと別の種類にするのは、両者が同時に開くことは無いにせよ、
	// 決定（DecidedMsg）の宛先を取り違えると「破棄するつもりの y で実行が始まる」
	// 形の事故になるためである。種類で分ければ取り違えは起きない。
	DiscardKind page.ModalKind = "setupdiscard"
)

// discardInput は入力の破棄を問う確認の中身を返す。
//
// 実行前プレビューの中身（何を作るか・何を実行するか）と違って**計画に依らない**
// ので、OpenDiscard がここで組む。呼び出し側は毎回同じ定型文を持たずに済む。
func discardInput() dialog.ConfirmInput {
	return dialog.ConfirmInput{
		Title:   "入力の破棄",
		Targets: nil,
		Impact:  []string{"入力した内容は保存されません"},
		Command: nil,
		Note:    nil,
	}
}

// confirmOpenMsg は確認ダイアログを開く指示。
type confirmOpenMsg struct{ input dialog.ConfirmInput }

// OpenConfirm は実行前プレビューを開く（FR-16）。中身は呼び出し側が組む。
func OpenConfirm(o *page.Overlay, in dialog.ConfirmInput) tea.Cmd {
	return o.Open(ConfirmKind, confirmOpenMsg{input: in})
}

// OpenDiscard は入力の破棄の確認を開く。
func OpenDiscard(o *page.Overlay) tea.Cmd {
	return o.Open(DiscardKind, confirmOpenMsg{input: discardInput()})
}

// confirmModal は確認ダイアログ。dialog.Confirm を包むだけで判断は持たない。
//
// 実行前プレビューと入力の破棄の 2 か所で使う。種類を値に持つのは、決定を
// 差し戻すときの ResultMsg.Kind を取り違えないためである。
type confirmModal struct {
	tab  int
	kind page.ModalKind
	dlg  dialog.Confirm
}

// tea.Model を実装していることをコンパイル時に確かめる。
var _ tea.Model = confirmModal{}

// NewConfirm は確認ダイアログのモーダルを組み立てる。画面は page.Overlay.Register
// に渡す。kind は ConfirmKind / DiscardKind のどちらかである。
func NewConfirm(st page.StateMsg, kind page.ModalKind) page.Modal {
	return page.Modal{
		Model: confirmModal{tab: 0, kind: kind, dlg: dialog.NewConfirm(st.Keys, st.Styles)},
		Title: confirmTitle,
		Hints: confirmHints,
		// esc を dialog.Confirm へ届ける。届かないと DecidedMsg{false} が出ず、
		// 承認待ちの計画が残ったまま次の確認で実行されうる。
		HandlesBack: func(tea.Model) bool { return true },
	}
}

// Init は何も発行しない。
func (m confirmModal) Init() tea.Cmd { return nil }

// Update は確認ダイアログへ Msg を配り、決定を page へ差し戻す。
func (m confirmModal) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case page.AttachMsg:
		m.tab = msg.Tab
		return m, nil
	case dialog.DecidedMsg:
		res := page.ResultMsg{Kind: m.kind, Msg: msg}
		return m, page.Do(m.tab, func() tea.Msg { return res })
	case confirmOpenMsg:
		m.dlg.SetInput(msg.input)
		return m, nil
	case page.StateMsg:
		m.dlg.Restyle(msg.Keys, msg.Styles)
		return m, nil
	case page.SizeMsg:
		m.dlg.SetSize(msg.W, msg.H)
		return m, nil
	default:
		var cmd tea.Cmd
		m.dlg, cmd = m.dlg.Update(msg)
		return m, page.WrapModal(m.tab, m.kind, cmd)
	}
}

// View は確認ダイアログの中身を返す。枠は Overlay が描く。
func (m confirmModal) View() tea.View { return tea.NewView(m.dlg.View()) }

// confirmTitle はモーダルの見出しを返す。
func confirmTitle(model tea.Model) string {
	m, ok := model.(confirmModal)
	if !ok {
		return ""
	}
	return m.dlg.Title()
}

// confirmHints はモーダルのフッタを返す。
func confirmHints(model tea.Model) []atom.Hint {
	m, ok := model.(confirmModal)
	if !ok {
		return nil
	}
	return m.dlg.Hints()
}
