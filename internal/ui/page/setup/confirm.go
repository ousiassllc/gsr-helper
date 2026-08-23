package setup

import (
	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/setup"
	"github.com/ousiassllc/gsr-helper/internal/ui/atom"
	"github.com/ousiassllc/gsr-helper/internal/ui/organism/dialog"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
)

// モーダルの種類。タブ名を接頭辞に付けるのは、Overlay が種類の重複で panic する
// ためである。
const (
	// confirmKind は実行前プレビュー。
	confirmKind page.ModalKind = "setupconfirm"
	// discardKind はフォームの入力を破棄してよいかの確認。
	//
	// 実行前プレビューと別の種類にするのは、両者が同時に開くことは無いにせよ、
	// 決定（DecidedMsg）の宛先を取り違えると「破棄するつもりの y で実行が始まる」
	// 形の事故になるためである。種類で分ければ取り違えは起きない。
	discardKind page.ModalKind = "setupdiscard"
)

// discardInput は入力の破棄を問う確認の中身を返す。
func discardInput() dialog.ConfirmInput {
	return dialog.ConfirmInput{
		Title:   "入力の破棄",
		Targets: nil,
		Impact:  []string{"入力した内容は保存されません"},
		Command: nil,
		Note:    nil,
	}
}

// confirmOpenMsg は実行前プレビューを開く指示。
type confirmOpenMsg struct{ input dialog.ConfirmInput }

// confirmInput は計画から確認ダイアログの中身を組み立てる（FR-16）。
//
// **計画をそのまま写す。** コマンド全文は setup.Plan が持つものを出し、UI で
// 組み直さない。組み直すと承認した文面と実際に発行される内容が食い違う
// （docs/ui/screens.md の確認ダイアログ）。トークンの位置は計画の時点から
// *** なので、ここでマスクし直す必要も無い。
func confirmInput(p setup.Plan) dialog.ConfirmInput {
	return dialog.ConfirmInput{
		Title:   p.Kind.String() + "の確認",
		Targets: targetLines(p),
		Impact:  p.Warnings,
		Command: commandLines(p),
		Note:    p.Notes,
	}
}

// targetLines は対象の行を返す。
//
// 追加はまだ存在しない runner なので、作成するディレクトリを対象として出す
// （screens.md の実行前の確認）。削除・更新は runner 名とスコープを出す。
func targetLines(p setup.Plan) []string {
	out := make([]string, 0, len(p.Units))
	for _, u := range p.Units {
		if p.Kind == setup.KindAdd {
			out = append(out, u.Dir)
			continue
		}
		out = append(out, u.Name+"  "+u.Runner.Scope.String())
	}
	return out
}

// commandLines は実行するコマンド全文を実行順に全件返す。
func commandLines(p setup.Plan) []string {
	out := make([]string, 0, len(p.Units)*4)
	for _, u := range p.Units {
		out = append(out, u.CommandLines()...)
	}
	return out
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

// newConfirmModal は確認ダイアログのモーダルを組み立てる。
func newConfirmModal(st page.StateMsg, kind page.ModalKind) page.Modal {
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
