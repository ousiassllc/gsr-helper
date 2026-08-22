package runnerdetail

import (
	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/appconfig"
	"github.com/ousiassllc/gsr-helper/internal/runner"
	"github.com/ousiassllc/gsr-helper/internal/ui/atom"
	"github.com/ousiassllc/gsr-helper/internal/ui/organism"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
)

// Kind は詳細画面のモーダルの種類。画面が page.Overlay へ登録するときに使う。
const Kind page.ModalKind = "runnerdetail"

// Open は詳細画面を開く。Runners / Jobs タブが共用する入口である。
//
// 種類と Msg の組を画面ごとに書かせないために用意する（取り違えると開かない、
// あるいは別のモーダルへ Msg が届く）。
//
// 戻り値の Cmd は呼び出し側まで返すこと。捨てると、開いた瞬間に処理を始める
// モーダルがその処理を動かせない（page.Overlay.Open の doc）。
func Open(o *page.Overlay, r runner.Runner, caps appconfig.Caps) tea.Cmd {
	return o.Open(Kind, OpenMsg{Runner: r, Caps: caps})
}

// OpenMsg は詳細画面を開く指示。対象の runner とそのときの能力を渡す。
//
// 開く指示を Msg にするのは、Overlay がモーダルの種類ごとの引数を知らずに済むように
// するためである（page.Modal の doc）。
type OpenMsg struct {
	Runner runner.Runner
	Caps   appconfig.Caps
}

// modal は runner の詳細画面のモーダル。Model を tea.Model として包む。
//
// Model 自身は bubbles 流の署名（具体型を返す Update と View() string）を保つ。
// tea.Model にすると呼び出し側で型アサーションが要り、詳細画面を直接組み立てて
// 検証する経路も回りくどくなるためである。
type modal struct {
	// tab は自分が乗っているタブ番号。page.AttachMsg で Overlay から受け取る。
	// 決定を page へ返す Cmd を包むために持つ（page.AttachMsg の doc）。
	tab    int
	detail Model
}

// tea.Model を実装していることをコンパイル時に確かめる。
var _ tea.Model = modal{}

// New は詳細画面のモーダルを組み立てる。画面は page.Overlay.Register に渡す。
func New(st page.StateMsg) page.Modal {
	return page.Modal{
		Model: modal{tab: 0, detail: newModel(st.Keys, st.Styles)},
		Title: title,
		Hints: hints,
		// esc は常に 1 枚閉じる。詳細画面には入力も編集も無く、esc に「戻る」以外の
		// 意味が無い（フッタも esc:戻る だけを出す。Model.Hints）。
		HandlesBack: nil,
	}
}

// Init は何も発行しない。開くタイミングは Overlay が決める。
func (m modal) Init() tea.Cmd { return nil }

// Update は開く指示・共有状態・大きさ・決定・キーを振り分ける。
func (m modal) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case page.AttachMsg:
		m.tab = msg.Tab
		return m, nil
	case organism.ChosenMsg:
		// 操作リストが返した決定を page へ差し戻す。ドメイン層を呼べるのは page
		// 階層だけ（atomic-design.md の依存の規則）であり、詳細画面はここで実行
		// できない。**page.Do で包む**ことでタブを切り替えても発行元の page へ戻り、
		// page.ResultMsg で包むことで Overlay が自分自身へ配り直さない。
		res := page.ResultMsg{Kind: Kind, Msg: msg}
		return m, page.Do(m.tab, func() tea.Msg { return res })
	case OpenMsg:
		m.detail.Open(msg.Runner, msg.Caps)
		return m, nil
	case page.StateMsg:
		m.detail.SetState(msg)
		return m, nil
	case page.SizeMsg:
		m.detail.SetSize(msg.W, msg.H)
		return m, nil
	default:
		var cmd tea.Cmd
		m.detail, cmd = m.detail.Update(msg)
		return m, cmd
	}
}

// View は情報部と操作リストを返す。
func (m modal) View() tea.View { return tea.NewView(m.detail.View()) }

// title はモーダルの見出しを返す。
func title(model tea.Model) string {
	m, ok := model.(modal)
	if !ok {
		return ""
	}
	return m.detail.Title()
}

// hints は詳細画面のフッタに出すキーヒントを返す。
func hints(model tea.Model) []atom.Hint {
	m, ok := model.(modal)
	if !ok {
		return nil
	}
	return m.detail.Hints()
}
