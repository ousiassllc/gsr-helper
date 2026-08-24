// Package configmodal は Config タブのモーダル 3 種——設定編集のフォーム・差分の
// 承認・反映方法の選択——を提供する。
//
// Config タブ（page/config）から分けているのは、**モーダルがタブの状態を 1 つも
// 見ないため**である。どれも organism/dialog と organism.ChoiceList を包み、開く
// 指示を受けて決定を page.ResultMsg で差し戻すだけで、判断（何を差分に載せるか・
// どの反映方法を並べるか・承認後に何を書くか）はすべてタブ側に残る。包み方の先例は
// page/progressmodal と page/disk/confirmmodal である。
//
// **置き場所は基準どおりではない。** 利用者は 1 タブだけなので既定ではネスト
// （page/<tab>/<名前>）だが、切り出しを指示した Issue が `shared` への登録を受け入れ
// 条件に含めていたため page/ 直下にある（atomic-design.md「`page/` は 1 ディレクトリ
// 1 タブではない」の例外）。**次にタブ 1 枚ぶんを切り出すときはネスト側に倣うこと。**
//
// 1 ディレクトリ 2000 行の上限（atomic-design.md「ディレクトリの行数」）に対しては、
// この 3 つと入力欄の組み立て（form.go）が Config タブで切れる最大の境界でもある。
package configmodal

import (
	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/config/edit"
	"github.com/ousiassllc/gsr-helper/internal/ui/atom"
	"github.com/ousiassllc/gsr-helper/internal/ui/organism"
	"github.com/ousiassllc/gsr-helper/internal/ui/organism/dialog"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
	"github.com/ousiassllc/gsr-helper/internal/ui/token"
)

// モーダルの種類。画面が page.Overlay へ登録するときに使う。
const (
	FormKind  page.ModalKind = "configform"
	DiffKind  page.ModalKind = "configdiff"
	ApplyKind page.ModalKind = "configapply"
)

// titleApply は反映方法の選択の見出し。
const titleApply = "反映方法を選んでください"

// OpenForm は設定編集のフォームを開く。
//
// 種類と Msg の組を画面ごとに書かせないために用意する（progressmodal.Open と同じ
// 理由）。戻り値の Cmd は呼び出し側まで返すこと。
func OpenForm(o *page.Overlay, title string, values *edit.Values, st page.StateMsg) tea.Cmd {
	return o.Open(FormKind, formOpenMsg{title: title, values: values, st: st})
}

// OpenDiff は差分の承認を開く（FR-37 / FR-38）。
func OpenDiff(o *page.Overlay, in dialog.DiffApprovalInput) tea.Cmd {
	return o.Open(DiffKind, diffOpenMsg{input: in})
}

// OpenApply は反映方法の選択を開く（FR-39）。並べる選択肢は呼び出し側が決める。
func OpenApply(o *page.Overlay, items []organism.Choice) tea.Cmd {
	return o.Open(ApplyKind, applyOpenMsg{items: items})
}

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

// NewForm はフォームのモーダルを組み立てる。画面は page.Overlay.Register に渡す。
func NewForm(st page.StateMsg) page.Modal {
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
		return m, page.WrapModal(m.tab, FormKind, cmd)
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
