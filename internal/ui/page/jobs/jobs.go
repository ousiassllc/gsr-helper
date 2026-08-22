// Package jobs は Jobs タブを実装する。
//
// このホストで実行中のジョブを runner 横断で一覧する（FR-04 の表示形態）。
// **操作対象はジョブではなく runner である**（FR-47）。取り違えを防ぐため、フッタでも
// 対象が runner であることを明示する。
//
// 検出は自分で行わず、親 Model から page.StateMsg で受け取ったスナップショットを
// 描画に使う（atomic-design.md の page の責務）。
package jobs

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/ui/atom"
	"github.com/ousiassllc/gsr-helper/internal/ui/organism/table"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/runnerdetail"
)

const (
	// inputFilter は入力中であることを状態行に出すときの名称（screens.md の入力中）。
	inputFilter = "絞り込み"
	// emptyMessage は実行中ジョブが無いときの表示（screens.md の Jobs タブ）。
	emptyMessage = "実行中のジョブはありません"
	// detailDesc は enter の説明（screens.md の Jobs タブのフッタ）。
	detailDesc = "runner の詳細"
)

// Model は Jobs タブ。
type Model struct {
	tab     int
	st      page.StateMsg
	tbl     table.Model[row]
	overlay page.Overlay
}

// tea.Model を実装していることをコンパイル時に確かめる。
var _ tea.Model = Model{}

// New は Jobs タブを組み立てる。tab は親が持つタブ番号で、ChromeMsg に載せる。
//
// モーダルは画面が登録する（page.Overlay の doc）。Jobs タブが開くのは runner の
// 詳細画面だけで、操作対象がジョブではなく runner であることと対応する（FR-47）。
func New(tab int, st page.StateMsg) Model {
	overlay := page.NewOverlay(st.Keys, st.Styles, st.Dark)
	overlay.Register(runnerdetail.Kind, runnerdetail.New(st))
	return Model{
		tab:     tab,
		st:      st,
		tbl:     newTable(st.Keys, st.Styles),
		overlay: overlay,
	}
}

// Init は初期化の Cmd を返す。検出は親が駆動するため、ここでは何も発行しない。
func (m Model) Init() tea.Cmd { return nil }

// Update は共有状態の反映とキー入力の解釈を行う。
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case page.StateMsg:
		return m.setState(msg)
	case tea.KeyPressMsg:
		return m.handleKey(msg)
	default:
		return m.forward(msg)
	}
}

// View はモーダルが開いていればそれを、無ければ一覧を返す。
func (m Model) View() tea.View {
	if m.overlay.Active() {
		return tea.NewView(m.overlay.View())
	}
	body := m.tbl.View()
	if body == "" {
		body = m.st.Styles.Muted.Render(emptyMessage)
	}
	return tea.NewView(body)
}

// setState は共有状態のスナップショットを反映する。ドメイン層は呼ばない。
func (m Model) setState(st page.StateMsg) (tea.Model, tea.Cmd) {
	m.st = st
	m.tbl.SetSize(st.BodyW, st.BodyH)
	m.tbl.SetItems(sectionJobs, jobRows(st.Result.Runners))
	m.overlay.SetState(st)
	return m, m.chrome()
}

// forward はキー以外の Msg を配る。モーダル表示中は最上位のモーダルにのみ渡す。
func (m Model) forward(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	if m.overlay.Active() {
		m.overlay, cmd = m.overlay.Update(msg)
		return m, tea.Batch(m.chrome(), cmd)
	}
	m.tbl, cmd = m.tbl.Update(msg)
	return m, tea.Batch(m.chrome(), cmd)
}

// handleKey はキー入力を解釈する。
//
// 入力中とモーダル表示中は**親へ差し戻さない**。グローバルキーを閉じ込められるのは
// この判定を持つ page だけである（page.GlobalKeyMsg の doc）。
func (m Model) handleKey(press tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch {
	case m.tbl.Filtering(), m.overlay.Active():
		return m.forward(press)
	case key.Matches(press, m.st.Keys.Global.Help):
		m.overlay.OpenHelp()
	case key.Matches(press, m.st.Keys.List.Enter):
		m.openDetail()
	case key.Matches(press, m.st.Keys.Global.Back):
		m.tbl.ClearFilter()
	default:
		// 自分が解釈しないキーは一覧へ渡し、同時に親へ差し戻す。タブ切替・再読み込み・
		// 終了を解釈するのは親であり、一覧のキーと衝突しないことは keymap の
		// TestNoDuplicateKeysInSameContext が担保する。
		next, cmd := m.forward(press)
		return next, tea.Batch(cmd, page.BubbleKey(press))
	}
	return m, m.chrome()
}

// openDetail はカーソル位置のジョブを実行している runner の詳細画面を開く（FR-47）。
//
// ジョブ単体を止める機能は持たない。ホスト側からは Runner.Worker を強制終了する
// しかなく、ジョブが失敗として記録されるためである（screens.md の Jobs タブ）。
func (m *Model) openDetail() {
	cur, ok := m.tbl.Selected()
	if !ok {
		return
	}
	runnerdetail.Open(&m.overlay, cur.runner, m.st.Caps)
}

// chrome は親へ本体以外の状態を知らせる Cmd を返す。
func (m Model) chrome() tea.Cmd {
	c := page.ChromeMsg{
		Tab:    m.tab,
		Modal:  m.overlay.Active(),
		Input:  m.input(),
		Status: m.status(),
		Footer: m.footer(),
	}
	return func() tea.Msg { return c }
}

// input は入力中の名称を返す。入力中でなければ空文字を返す。
func (m Model) input() string {
	if m.tbl.Filtering() {
		return inputFilter
	}
	return ""
}

// status は状態行に出す page 側の文を返す。
//
// 選択件数は出さない。Jobs タブは複数選択して一括操作する画面ではなく（操作対象は
// カーソル位置のジョブを実行している runner 1 台）、区画も選択できないためである。
func (m Model) status() string {
	if in := m.input(); in != "" {
		return "入力中: " + in
	}
	return ""
}

// footer はフッタのキーヒントを返す。
//
// 出す操作と表記は keymap.RunnerKeys.JobsFooter に従い、Jobs タブ側では持たない。
// キーと説明文の出どころを 1 つにするためである（page が文言を書き直すと、キーを
// 差し替えたときにこのフッタだけが古くなる）。可否と理由の判定も page.Allowed に
// 任せ、同じ操作の理由が Runners タブと食い違わないようにする。
//
// 先頭の `enter:runner の詳細` は、操作対象がジョブではなく runner であることの明示を
// 兼ねる（FR-47）。全キーに「runner を」を付けると幅 80 に収まらず、有効なキーが
// フッタから落ちる（設計原則 1 に反する）。
func (m Model) footer() []atom.Hint {
	if m.overlay.Active() {
		return m.overlay.Hints()
	}
	cur, ok := m.tbl.Selected()
	if !ok {
		return nil
	}

	keys := m.st.Keys.Runner
	footer := keys.JobsFooter()
	hints := make([]atom.Hint, 0, len(footer)+1)
	hints = append(hints, atom.Hint{
		Key:     page.BindingKey(m.st.Keys.List.Enter),
		Desc:    detailDesc,
		Enabled: true,
		Reason:  "",
	})
	for _, f := range footer {
		k := page.BindingKey(f.Binding)
		enabled, reason := page.Allowed(k, cur.runner, m.st.Caps, keys)
		hints = append(hints, atom.Hint{
			Key:     k,
			Desc:    f.Desc,
			Enabled: enabled,
			Reason:  reason,
		})
	}
	return hints
}
