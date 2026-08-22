// Package runners は Runners タブ（既定画面）を実装する。
//
// ホスト上の runner 一覧と孤児ユニットを 2 区画で表示し、enter で詳細画面を開く。
// 検出は自分で行わず、親 Model から page.StateMsg で受け取ったスナップショットを
// 描画に使う（atomic-design.md の page の責務）。
package runners

import (
	"strconv"

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
	// emptyMessage は runner が 1 台も見つからないときの表示。
	emptyMessage = "runner が見つかりません（--root で走査ルートを追加できます）"
)

// Model は Runners タブ。
type Model struct {
	tab     int
	st      page.StateMsg
	tbl     table.Model[row]
	overlay page.Overlay
}

// tea.Model を実装していることをコンパイル時に確かめる。
var _ tea.Model = Model{}

// New は Runners タブを組み立てる。tab は親が持つタブ番号で、ChromeMsg に載せる。
//
// モーダルは画面が登録する。重なりの規則（キーは最上位だけ・esc は 1 枚）は
// page.Overlay が種類に依らず担保するので、タブを足す Issue は自分のモーダルを
// Register するだけで済む。
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
//
// 本体に重ねる合成はしない（template.Modal の方式に合わせる）。
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

// setState は共有状態のスナップショットを反映する。
//
// ドメイン層は呼ばない。返すのは親へ本体以外の状態を知らせる ChromeMsg の Cmd だけで、
// 検出は親が 1 本の Cmd で駆動する（同じ検出が重複実行されないようにするため）。
func (m Model) setState(st page.StateMsg) (tea.Model, tea.Cmd) {
	m.st = st
	m.tbl.SetSize(st.BodyW, st.BodyH)
	m.tbl.SetItems(sectionRunners, runnerRows(st.Result.Runners))
	m.tbl.SetItems(sectionOrphans, orphanRows(st.Result.OrphanUnits))
	// モーダルが返す Cmd も親へ渡す（開いているモーダルが共有状態を受けて
	// 何かを始めることがある。捨てるとその処理が動かない）。
	cmd := m.overlay.SetState(st)
	return m, tea.Batch(m.chrome(), cmd)
}

// forward はキー以外の Msg を配る。
//
// モーダルが開いている間は最上位のモーダルにのみ渡す。開いていなければ一覧へ渡す
// （絞り込みのカーソル点滅など、organism/table が発行を続ける Cmd を止めないため）。
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
// この判定を持つ page だけであり（page.GlobalKeyMsg の doc）、ここで差し戻すと
// 確認中に打った q でアプリが終わる。
func (m Model) handleKey(press tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch {
	case m.tbl.Filtering(), m.overlay.Active():
		return m.forward(press)
	case key.Matches(press, m.st.Keys.Global.Help):
		m.overlay.OpenHelp()
	case key.Matches(press, m.st.Keys.List.Enter):
		m.openDetail()
	case key.Matches(press, m.st.Keys.Global.Back):
		m.back()
	default:
		// 自分が解釈しないキーは一覧へ渡し、同時に親へ差し戻す。タブ切替・再読み込み・
		// 終了を解釈するのは親であり、一覧のキーと衝突しないことは keymap の
		// TestNoDuplicateKeysInSameContext が担保する。
		next, cmd := m.forward(press)
		return next, tea.Batch(cmd, page.BubbleKey(press))
	}
	return m, m.chrome()
}

// openDetail はカーソル位置の runner の詳細画面を開く。
//
// 孤児ユニットの行では開かない。孤児ユニットには対応する runner ディレクトリが
// 無く、詳細画面の項目（スコープ・バージョン・ディレクトリ）を埋められないためである。
func (m *Model) openDetail() {
	cur, ok := m.tbl.Selected()
	if !ok || cur.isOrphan {
		return
	}
	runnerdetail.Open(&m.overlay, cur.runner, m.st.Caps)
}

// back は esc の「選択のクリア / 1 つ前の状態へ戻る」を処理する。
//
// 選択を先に解くのは、選択したまま絞り込みを解除すると画面外の対象が選択されたまま
// 残るためである（screens.md のグローバルキー）。
func (m *Model) back() {
	if len(m.tbl.Checked()) > 0 {
		m.tbl.ClearSelection()
		return
	}
	m.tbl.ClearFilter()
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
// 孤児ユニット件数と警告件数は親が出す（検出結果を持っているのは親であり、
// タブが変わっても同じ件数を出すため）。ここでは page しか知らない選択件数と
// 入力中を返す。
func (m Model) status() string {
	if in := m.input(); in != "" {
		return "入力中: " + in
	}
	if n := len(m.tbl.Checked()); n > 0 {
		return "選択: " + strconv.Itoa(n) + " 件"
	}
	return ""
}

// footer はフッタのキーヒントを返す。
//
// モーダル表示中はそのモーダルのキー、孤児ユニットの区画では一覧の移動キー、
// runner の行ではカーソル位置の runner に対する操作の可否を出す。
func (m Model) footer() []atom.Hint {
	if m.overlay.Active() {
		return m.overlay.Hints()
	}
	cur, ok := m.tbl.Selected()
	if !ok || cur.isOrphan {
		return m.listHints()
	}
	return page.Hints(cur.runner, m.st.Caps, m.st.Keys.Runner)
}

// listHints は操作の対象が無いときのフッタを返す。
//
// 孤児ユニットに対する操作はこの版に無く、enter でも詳細を開けないため、一覧の
// 移動と絞り込みだけを出す。キーと説明は keymap から取り、フッタとヘルプで文言が
// 食い違わないようにする。
//
// キー表記は page.BindingKey で作る。Help().Key は "j/↓" のように別名を含む表記で、
// 詳細画面のフッタ（page.RunnerDetail.Hints）は "j/k" なので、同じフッタの中で
// 移動キーの表記が 2 通りになるためである。
func (m Model) listHints() []atom.Hint {
	l := m.st.Keys.List
	return []atom.Hint{
		{
			Key:     page.BindingKey(l.Down) + "/" + page.BindingKey(l.Up),
			Desc:    "移動",
			Enabled: true,
			Reason:  "",
		},
		{Key: page.BindingKey(l.Filter), Desc: l.Filter.Help().Desc, Enabled: true, Reason: ""},
	}
}
