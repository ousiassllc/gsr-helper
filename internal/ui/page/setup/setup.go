// Package setup は Setup タブ（runner の追加・削除・バージョン更新）を提供する。
//
// 画面は「メニュー → フォーム → 実行前プレビュー → 進捗 → 結果報告」と進む
// （docs/ui/screens.md の Setup タブ）。計画（setup.Plan）を組むのはドメイン層で、
// この層は計画をそのまま表示し、承認を得てから同じ計画を実行に渡すだけである。
// 表示と実行の出どころが 1 つなので、承認した内容と実際に走る内容が食い違わない。
//
// **確認を経ない破壊的経路を作らない。** setup.Apply を呼ぶのは startRun 1 か所で、
// startRun を呼ぶのは dialog.DecidedMsg{Confirmed: true} を受けた onResult
// 1 か所だけである（page/disk のクリーンアップと同じ構造）。
package setup

import (
	"context"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/runner"
	"github.com/ousiassllc/gsr-helper/internal/runner/scope"
	"github.com/ousiassllc/gsr-helper/internal/setup"
	"github.com/ousiassllc/gsr-helper/internal/ui/keymap"
	"github.com/ousiassllc/gsr-helper/internal/ui/molecule"
	"github.com/ousiassllc/gsr-helper/internal/ui/organism"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/action"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/progressmodal"
)

// runState は実行中の 1 件。
//
// **ポインタで持つ。** 値で持つと Update のたびに写しが分岐し、進捗の取り込み先が
// 実行中の状態と別物になる（page/runnerop の drainRun と同じ理由）。
type runState struct {
	seq    int
	cancel context.CancelFunc
	ch     <-chan setup.Progress
	rows   []molecule.ProgressView
	done   int
	total  int
	// title は状態行に出す件数つきの見出し（「追加中… 2/3」）。
	title string
	// bare は進捗表示へ渡す件数抜きの見出し（「追加中…」）。
	//
	// pane.ProgressList が Done/Total を自分で添えるため、件数つきを渡すと
	// 二重に出る（chrome.go の bareTitle）。title から削り直すのではなく
	// 最初から 2 つ持つ。書式を変えたときに削る側が置いていかれない。
	bare string
	// kind は進捗の見出しに出す操作名（「追加」「削除」「バージョン更新」）。
	kind string
}

// Model は Setup タブ。
type Model struct {
	tab     int
	st      page.StateMsg
	menu    organism.ChoiceList
	actions action.Set
	overlay page.Overlay
	// initCmd は modal の登録が返した Cmd。最初の StateMsg で親へ流す。
	initCmd tea.Cmd

	// vals はフォームの入力先。huh がポインタで束縛するため実体を持ち続ける。
	vals *formValues

	plan     setup.Plan
	apiScope scope.Scope
	pending  page.SetupOp
	targets  []runner.Runner
	run      *runState
	report   []string
	notice   string
	seq      int
	// waiting は計画の組み立て（API 呼び出し）が走っているか。
	waiting bool
	// formShown はフォームを表示中か。状態行の「入力中」の判定に使う。
	formShown bool
}

// tea.Model を実装していることをコンパイル時に確かめる。
var _ tea.Model = Model{}

// New は Setup タブを組み立てる。
func New(tab int, st page.StateMsg) Model {
	overlay, help := page.NewOverlay(tab, st)
	form := overlay.Register(formKind, newFormModal(st))
	confirm := overlay.Register(confirmKind, newConfirmModal(st, confirmKind))
	discard := overlay.Register(discardKind, newConfirmModal(st, discardKind))
	progress := overlay.Register(progressmodal.Kind, progressmodal.New(st))
	scopeCmd := overlay.SetHelpScope(keymap.Set.SetupHelp)

	m := Model{
		tab: tab, st: st,
		menu:    organism.NewChoiceList(st.Keys.List, st.Styles),
		actions: action.NewSet(st.Keys.Runner, st.Scopes),
		overlay: overlay,
		initCmd: tea.Batch(help, form, confirm, discard, progress, scopeCmd),
		vals:    newValues(st),
		plan:    setup.Plan{}, apiScope: scope.Scope{Kind: scope.Unknown, Owner: "", Repo: ""},
		pending: page.SetupAdd, targets: nil, run: nil,
		report: nil, notice: "", seq: 0, waiting: false, formShown: false,
	}
	m.setMenu(organism.ResetCursor)
	return m
}

// Init は何も発行しない。親はタブの Init を呼ばず、最初の StateMsg で駆動する。
func (m Model) Init() tea.Cmd { return nil }

// Update は Msg を種類ごとに振り分ける。
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case page.StateMsg:
		return m.setState(msg)
	case tea.KeyPressMsg:
		return m.handleKey(msg)
	case page.ResultMsg:
		cmd := m.onResult(msg)
		return m, tea.Batch(m.chrome(), cmd)
	case organism.ChosenMsg:
		cmd := m.onChosen(msg)
		return m, tea.Batch(m.chrome(), cmd)
	case page.SetupRequestMsg:
		cmd := m.onRequest(msg)
		return m, tea.Batch(m.chrome(), cmd)
	case planMsg:
		cmd := m.onPlan(msg)
		return m, tea.Batch(m.chrome(), cmd)
	case progressMsg:
		cmd := m.onProgress(msg)
		return m, tea.Batch(m.chrome(), cmd)
	case doneMsg:
		cmd := m.onDone(msg)
		return m, tea.Batch(m.chrome(), cmd)
	case page.ActivateMsg:
		return m, m.chrome()
	case page.DeactivateMsg:
		return m, m.chrome()
	case page.ShutdownMsg:
		m.stopRun()
		return m, m.chrome()
	default:
		return m.forwardTo(msg)
	}
}

// setState は共有状態を受けて表示を組み直す。
func (m Model) setState(st page.StateMsg) (tea.Model, tea.Cmd) {
	m.st = st
	m.menu.Restyle(st.Keys.List, st.Styles)
	m.menu.SetWidth(st.BodyW)
	m.actions = action.NewSet(st.Keys.Runner, st.Scopes)
	m.vals.applyDefaults(st)
	m.setMenu(organism.KeepCursor)

	// 登録の Cmd は return より前に取り出す。同じ return 文に置くと、値レシーバの
	// 写しに対して評価順が入れ替わり、初回の登録が親へ届かないことがある。
	init := m.flushInit()
	state := m.overlay.SetState(st)
	scopeCmd := m.overlay.SetHelpScope(keymap.Set.SetupHelp)

	return m, tea.Batch(m.chrome(), init, state, scopeCmd)
}

// flushInit は登録の Cmd を 1 度だけ返す。
func (m *Model) flushInit() tea.Cmd {
	cmd := m.initCmd
	m.initCmd = nil
	return cmd
}

// handleKey はキー入力を処理する。
func (m Model) handleKey(press tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	m.notice = ""

	var cmd tea.Cmd
	switch {
	case m.overlay.Active():
		return m.forwardTo(press)
	case key.Matches(press, m.st.Keys.Global.Help):
		cmd = m.overlay.OpenHelp()
	case key.Matches(press, m.st.Keys.Runner.Add):
		cmd = m.startAdd(bulkForm)
	case key.Matches(press, m.st.Keys.Runner.Update):
		cmd = m.startUpdate(m.allRunners())
	case key.Matches(press, m.st.Keys.Runner.Delete):
		m.notice = noticeDeleteFromList
	case key.Matches(press, m.st.Keys.Global.Back):
		// 戻り先が一意に決まるので esc で Runners へ戻る（screens.md の画面遷移）。
		m.report = nil
		cmd = page.OpenTab(page.TabRunners, nil)
	default:
		next, c := m.forwardTo(press)
		return next, tea.Batch(c, page.BubbleKey(press))
	}
	return m, tea.Batch(m.chrome(), cmd)
}

// forwardTo はモーダルかメニューへ Msg を配る。
func (m Model) forwardTo(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	if m.overlay.Handles(msg) {
		m.overlay, cmd = m.overlay.Update(msg)
		return m, tea.Batch(m.chrome(), cmd)
	}

	m.menu, cmd = m.menu.Update(msg)
	return m, tea.Batch(m.chrome(), m.wrapMenu(cmd))
}

// wrapMenu はメニューが返した Cmd の結果をこのタブへ戻す。
//
// 包まないと結果は「そのとき選択中のタブ」へ配られる（page.TabMsg の doc）。
// 決定（organism.ChosenMsg）が届くまでの間にタブを切り替えられるため、
// 包まずに流すと別のタブが決定を受け取りうる。
func (m Model) wrapMenu(cmd tea.Cmd) tea.Cmd {
	if cmd == nil {
		return nil
	}
	return page.Do(m.tab, func() tea.Msg { return cmd() })
}

// View はモーダルがあればそれを、無ければメニューを描く。
func (m Model) View() tea.View {
	if m.overlay.Active() {
		return tea.NewView(m.overlay.View())
	}

	body := m.st.Styles.Header.Render(headMenu) + "\n\n" + m.menu.View()
	if len(m.report) > 0 {
		body += "\n\n" + m.reportView()
	}
	return tea.NewView(body)
}

// stopRun は実行中の処理を畳む。
func (m *Model) stopRun() {
	if m.run == nil {
		return
	}
	m.run.cancel()
	m.run = nil
}
