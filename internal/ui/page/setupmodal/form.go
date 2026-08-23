// Package setupmodal は Setup タブのモーダル——追加フォームと確認ダイアログ
// （実行前プレビュー / 入力の破棄）——を提供する。
//
// Setup タブ（page/setup）から分けているのは、**モーダルがタブの状態を 1 つも
// 見ないため**である。どちらも organism/dialog を包み、開く指示を受けて完了・中断・
// 決定を page.ResultMsg で差し戻すだけで、判断（何を入力させるか・何を確認に載せるか・
// 承認後に何を実行するか）はタブ側に残る。先例は page/progressmodal と
// page/disk/confirmmodal である。
//
// 1 ディレクトリ 2000 行の上限（atomic-design.md「ディレクトリの行数」）に対しては、
// Setup タブで残っていた唯一の手でもある（同書「`ui/page/setup` が警告帯に入った判断」）。
package setupmodal

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"

	"github.com/ousiassllc/gsr-helper/internal/ui/atom"
	"github.com/ousiassllc/gsr-helper/internal/ui/organism/dialog"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
	"github.com/ousiassllc/gsr-helper/internal/ui/token"
)

// FormKind は追加フォームのモーダルの種類。
const FormKind page.ModalKind = "setupform"

// formOpenMsg は追加フォームを開く指示。
//
// 組み立て済みの huh.Form ではなく**組み立てる関数**を受けるのは、テーマ（配色）の
// 決め方をモーダル側に残すためである。呼び出し側は「何を入力させるか」だけを持つ。
type formOpenMsg struct {
	title string
	build func(huh.Theme) *huh.Form
	st    page.StateMsg
}

// OpenForm は追加フォームを開く。
//
// 種類と Msg の組を画面ごとに書かせないために用意する（progressmodal.Open と同じ
// 理由）。戻り値の Cmd は呼び出し側まで返すこと。
func OpenForm(o *page.Overlay, st page.StateMsg, title string, build func(huh.Theme) *huh.Form) tea.Cmd {
	return o.Open(FormKind, formOpenMsg{title: title, build: build, st: st})
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

// NewForm は追加フォームのモーダルを組み立てる。画面は page.Overlay.Register に渡す。
func NewForm(st page.StateMsg) page.Modal {
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
		res := page.ResultMsg{Kind: FormKind, Msg: msg}
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
		return m, page.WrapModal(m.tab, FormKind, cmd)
	}
}

// open は渡された組み立て関数でフォームを作って表示する。
func (m formModal) open(msg formOpenMsg) (tea.Model, tea.Cmd) {
	m.color = msg.st.Color
	m.form.SetTitle(msg.title)
	cmd := m.form.SetForm(msg.build(token.HuhTheme(msg.st.Styles, m.color)))
	return m, page.WrapModal(m.tab, FormKind, cmd)
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
