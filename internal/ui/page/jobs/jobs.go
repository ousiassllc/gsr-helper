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
func New(tab int, st page.StateMsg) Model {
	return Model{
		tab:     tab,
		st:      st,
		tbl:     newTable(st.Keys, st.Styles),
		overlay: page.NewOverlay(st.Keys, st.Styles, st.Dark),
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

// handleKey はキー入力を解釈する。入力中とモーダル表示中はそのまま配る。
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
		return m.forward(press)
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
	m.overlay.OpenDetail(cur.runner, m.st.Caps)
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

// footerKey は Jobs タブのフッタ 1 項目のキーと文言。
type footerKey struct {
	key  string
	desc string
}

// footerKeys は Jobs タブのフッタに出す操作と文言を返す（screens.md の Jobs タブ）。
//
// 操作対象がジョブではなく runner であることを画面上で明示する（FR-47）が、
// **明示は先頭の `enter:runner の詳細` に代表させる**。全キーに「runner を」を
// 付けると幅 80 に収まらず、有効なキーがフッタから落ちる（設計原則 1 に反する）。
//
// 出す操作は screens.md の Jobs タブが挙げる 4 つ（ドレイン・強制停止・再起動・
// ログ）に限る。削除と設定編集は詳細画面から行う。文言は Runners タブのフッタと
// 同じく短い表記を使う（説明は ? の全キー一覧が担う）。
func footerKeys() []footerKey {
	return []footerKey{
		{key: "d", desc: "ドレイン"},
		{key: "X", desc: "強制停止"},
		{key: "R", desc: "再起動"},
		{key: "l", desc: "ログ"},
	}
}

// footer はフッタのキーヒントを返す。
//
// 可否と理由の判定は page.Allowed に任せ、Jobs タブ側で操作可否を作り直さない。
// 同じ操作の理由が Runners タブと食い違わないようにするためである。
func (m Model) footer() []atom.Hint {
	if m.overlay.Active() {
		return m.overlay.Hints()
	}
	cur, ok := m.tbl.Selected()
	if !ok {
		return nil
	}

	keys := footerKeys()
	hints := make([]atom.Hint, 0, len(keys)+1)
	hints = append(hints, atom.Hint{
		Key:     page.BindingKey(m.st.Keys.List.Enter),
		Desc:    detailDesc,
		Enabled: true,
		Reason:  "",
	})
	for _, f := range keys {
		enabled, reason := page.Allowed(f.key, cur.runner, m.st.Caps)
		hints = append(hints, atom.Hint{
			Key:     f.key,
			Desc:    f.desc,
			Enabled: enabled,
			Reason:  reason,
		})
	}
	return hints
}
