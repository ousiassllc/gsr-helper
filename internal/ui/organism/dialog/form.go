package dialog

import (
	"reflect"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"

	"github.com/ousiassllc/gsr-helper/internal/ui/atom"
	"github.com/ousiassllc/gsr-helper/internal/ui/keymap"
	"github.com/ousiassllc/gsr-helper/internal/ui/token"
)

// FormDoneMsg は入力の完了を page へ通知する。
//
// 入力値そのものではなくフォームを載せるのは、値を持っているのが huh.Form 自身
// （Get / GetString / GetInt / GetBool）だからである。写しを作ると、huh の Value による
// 変数への束縛と 2 つの取り出し口ができ、どちらが正なのか分からなくなる。
//
// **page はこれを受けてドメイン層の Cmd を発行する。Form 自身はドメインを呼ばない**
// （atomic-design.md の「`Form` と huh」）。
type FormDoneMsg struct {
	Form *huh.Form
}

// FormAbortedMsg は中断の確定を page へ通知する。
//
// 入力が空のまま esc を押した場合と、huh 自身が中断した場合（ctrl+c）に届く。
// 破棄してよいことが決まっている状態なので、page はそのまま前の画面へ戻る。
type FormAbortedMsg struct{}

// FormDiscardMsg は破棄の確認が要ることを page へ通知する。
//
// 入力済みの項目がある状態で esc を押すと届く。**確認そのものはここで行わない。**
// 確認ダイアログの実装は Confirm 1 つに統一する決まりであり（doc.go）、Form の中に
// もう 1 つ持つと「確認を経ない破棄」の経路が別実装として増える。page が Confirm を
// 重ね、その結果で戻るか入力へ返すかを決める。
type FormDiscardMsg struct{}

// Form は huh.Form のラッパー。
//
// huh はキー処理・検証・レイアウトを自分で持つため、責務は 3 点に限る
// （atomic-design.md の「`Form` と huh」）。
//
//   - token が組み立てたテーマを適用する（フォームだけ配色が浮かないようにする）
//   - 完了・中断を tea.Msg で page へ返す（ドメイン層は呼ばない）
//   - esc を受けたとき、入力済みなら破棄の確認を求め、空なら即座に戻る
//
// **キーヒントはフォーム自身に描かせる。** 有効なキーは項目の種類（入力欄・選択・
// 複数選択）で変わり、それを知っているのは huh だけである。Hints が返すのは、huh が
// 知らないこのラッパー自身のキー（esc）だけにする。
type Form struct {
	form   *huh.Form
	title  string
	back   key.Binding
	styles token.Styles
	color  bool
	dirty  bool
	width  int
	height int
}

// NewForm は空のフォームのラッパーを組み立てる。中身は SetForm で入れる。
//
// 色の有効無効を Styles と別に受け取るのは、huh.Theme の組み立てが NO_COLOR の
// 指定を Styles より優先するためである（token.HuhTheme）。
//
// esc のキー定義を引数に取らないのは、戻るキーが画面ごとに変わらないためである
// （pane.NewDetail が keymap から読むのと同じ）。
func NewForm(s token.Styles, color bool) Form {
	return Form{
		form:   nil,
		title:  "",
		back:   keymap.NewGlobal().Back,
		styles: s,
		color:  color,
		dirty:  false,
		width:  0,
		height: 0,
	}
}

// SetForm は表示するフォームを差し替え、初期化の Cmd を返す。
//
// テーマ・大きさと、完了・中断の通知を差し替えたフォームへ与え直す。**入力済みの印は
// ここで落とす。** 別のフォームを開いたときに前回の入力状態が残っていると、1 文字も
// 打っていない画面で esc が破棄の確認を出す。
//
// 渡すフォームは項目を 1 つ以上持つこと。huh は空のフォームの焦点を取れず、
// 中身を触った時点で panic する（huh 側の前提であり、ここでは検査しない）。
func (f *Form) SetForm(form *huh.Form) tea.Cmd {
	f.form, f.dirty = form, false
	if form == nil {
		return nil
	}

	// 完了・中断の通知は huh の Cmd 差込口に預ける。状態（huh.FormState）を毎回
	// 見に行くと、通知済みかどうかをこちら側でも数えることになる。
	form.SubmitCmd = func() tea.Msg { return FormDoneMsg{Form: form} }
	form.CancelCmd = func() tea.Msg { return FormAbortedMsg{} }
	f.apply()
	return form.Init()
}

// Restyle は配色を差し替える。入力の内容と入力済みの印は保つ。
//
// 作り直さずに差し替えるのは、背景の明暗が起動後に届き（tea.BackgroundColorMsg）、
// 共有状態が 3 秒ごとに配られるためである（Confirm.Restyle と同じ理由）。作り直すと
// 入力の途中で打った内容が消える。
func (f *Form) Restyle(s token.Styles, color bool) {
	f.styles, f.color = s, color
	f.apply()
}

// SetSize はフォームに配られた領域を設定する。
func (f *Form) SetSize(w, h int) {
	f.width, f.height = w, h
	f.apply()
}

// SetTitle は見出しを差し替える。枠（template.Modal）が描く。
//
// フォームの中の見出し（huh.Group の Title）と別に持つのは、モーダルの見出しを描くのが
// 枠であり、huh はその外側を知らないためである。
func (f *Form) SetTitle(title string) {
	f.title = title
}

// apply はテーマと大きさを今のフォームへ与える。
func (f *Form) apply() {
	if f.form == nil {
		return
	}
	f.form = f.form.WithTheme(token.HuhTheme(f.styles, f.color))
	if f.width > 0 {
		f.form = f.form.WithWidth(f.width)
	}
	if f.height > 0 {
		f.form = f.form.WithHeight(f.height)
	}
}

// Title は見出しを返す。枠（template.Modal）が描くため View には含めない。
func (f Form) Title() string { return f.title }

// Dirty は入力済みの項目があるかを返す。
func (f Form) Dirty() bool { return f.dirty }

// Update は esc を解釈し、それ以外を huh へ配る。
//
// **esc は huh へ渡さない。** huh の既定では esc に割り当てが無く、渡しても中断には
// ならない。破棄の可否を問うかどうかはこのラッパーの責務なので、ここで止める。
// esc をここで解釈するため、page はこの画面の Modal.HandlesBack に真を返させること
// （Confirm.Update / DrainWaiter.Update と同じ）。
func (f Form) Update(msg tea.Msg) (Form, tea.Cmd) {
	if f.form == nil {
		return f, nil
	}

	if press, ok := msg.(tea.KeyPressMsg); ok && key.Matches(press, f.back) {
		if f.dirty {
			return f, func() tea.Msg { return FormDiscardMsg{} }
		}
		return f, func() tea.Msg { return FormAbortedMsg{} }
	}

	before := f.focused()
	model, cmd := f.form.Update(msg)
	if form, ok := model.(*huh.Form); ok {
		f.form = form
	}
	f.dirty = f.dirty || changed(before, f.focused())
	return f, cmd
}

// View はフォームの中身を返す。見出しは枠が描くため含めない。
func (f Form) View() string {
	if f.form == nil {
		return ""
	}
	return f.form.View()
}

// Hints はフッタに出すキーヒントを返す。
//
// 返すのは esc だけである。項目の移動・確定・選択のキーは項目の種類で変わり、それを
// 描けるのは huh 自身だけなので、フォームの中の案内に任せる。ここへ書き写すと、
// 同じキーがフッタとフォームの 2 か所に、しかも別々の表記で並ぶ。
//
// 入力済みのときだけ説明を「破棄して戻る」に変える。**確認が挟まることは押す前に
// 分かる方がよい**（設計原則 5 の「破壊的操作は影響を表示する」と同じ考え）。
func (f Form) Hints() []atom.Hint {
	if f.dirty {
		return []atom.Hint{hint(f.back, "破棄して戻る")}
	}
	return []atom.Hint{hint(f.back, "戻る")}
}

// fieldValue は焦点のある項目の識別子と値。入力済みかどうかの判定に使う。
type fieldValue struct {
	key   string
	value any
}

// focused は焦点のある項目の識別子と値を返す。
func (f Form) focused() fieldValue {
	field := f.form.GetFocusedField()
	return fieldValue{key: field.GetKey(), value: field.GetValue()}
}

// changed は同じ項目の値が変わったかを返す。
//
// **焦点のある項目 1 つだけを見る。** huh は全項目の値をまとめて読み出す手立てを
// 公開しておらず（Get は識別子を知っている呼び出し側のためのもの）、項目の一覧を
// 辿るには内部構造に触れるしかない。値を変えるには必ずその項目に焦点が要るので、
// 1 打鍵ごとに焦点のある項目を見比べれば、どの項目への入力も取りこぼさない。
//
// 識別子が違う場合は項目の移動なので変化とみなさない。移動そのものは入力ではなく、
// 既定値の違う項目へ移っただけで「入力済み」になると、素通りしただけの画面で
// esc が破棄の確認を出す。
func changed(before, after fieldValue) bool {
	if before.key != after.key {
		return false
	}
	// 値の型は項目によって違い、複数選択は比較できないスライスを返す。== では panic する。
	return !reflect.DeepEqual(before.value, after.value)
}
