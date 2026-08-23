package runnerop

import (
	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/runner"
	"github.com/ousiassllc/gsr-helper/internal/ui/atom"
	"github.com/ousiassllc/gsr-helper/internal/ui/organism/dialog"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
)

// DrainKind はドレイン待機画面のモーダルの種類。
const DrainKind page.ModalKind = "drain"

// titleGap は見出しと添える進捗の間隔（screens.md のモックの見出しに合わせる）。
const titleGap = "   "

// drainModal はドレイン待機画面を page.Modal として包む。包み方は confirmModal と同じ。
type drainModal struct {
	// tab は自分が乗っているタブ番号。page.AttachMsg で Overlay から受け取る。
	tab int
	dlg dialog.DrainWaiter
	// target は待機中の runner。3 秒ごとの共有状態から Dir で引き直す。
	//
	// 名前ではなくディレクトリで引くのは、Dir が検出の識別子（一覧の行の ID も
	// これである）だからである。名前は config.sh の設定次第で重複しうる。
	target runner.Runner
	// label は見出しに添える進捗（"(2/3)"）。
	label string
	// started は計時とスピナを動かしたか。
	//
	// 2 件目以降の対象で Start を呼び直すと、スピナの Tick の連鎖が二重になって
	// 回転が倍速になる（自分の Tick を Update で繋いで回るため。
	// dialog.DrainWaiter.Stop の doc）。経過時間は「待ち始めてからの時間」であり
	// 対象が変わっても継続してよいので、動かすのは最初の 1 回だけでよい。
	started bool
}

// tea.Model を実装していることをコンパイル時に確かめる。
var _ tea.Model = drainModal{}

// NewDrainModal はドレイン待機画面のモーダルを組み立てる。
func NewDrainModal(st page.StateMsg) page.Modal {
	return page.Modal{
		Model: drainModal{
			tab:     0,
			dlg:     dialog.NewDrainWaiter(st.Keys.Global, st.Styles),
			target:  runner.Runner{},
			label:   "",
			started: false,
		},
		Title: drainTitle,
		Hints: drainHints,
		// esc は待機画面自身がキャンセルとして解釈する（confirmModal と同じ理由）。
		// 真を返さないと Overlay が esc を「1 枚閉じる」で消費し、画面だけが閉じて
		// **待機は goroutine の中で走り続ける**（キャンセルの context が呼ばれない）。
		HandlesBack: func(tea.Model) bool { return true },
	}
}

// Init は何も発行しない。開くタイミングは Overlay が決める。
func (m drainModal) Init() tea.Cmd { return nil }

// Update は開く指示・停止の指示・共有状態・大きさ・キャンセル・Tick を振り分ける。
func (m drainModal) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case page.AttachMsg:
		m.tab = msg.Tab
		return m, nil
	case dialog.DrainCanceledMsg:
		// キャンセルを page へ差し戻す（confirmModal と同じ理由）。
		res := page.ResultMsg{Kind: DrainKind, Msg: msg}
		return m, page.Do(m.tab, func() tea.Msg { return res })
	case drainOpenMsg:
		return m.open(msg)
	case drainStopMsg:
		m.started = false
		return m, wrap(m.tab, DrainKind, m.dlg.Stop())
	case page.StateMsg:
		m.dlg.Restyle(msg.Keys.Global, msg.Styles)
		m.refresh(msg)
		return m, nil
	case page.SizeMsg:
		m.dlg.SetSize(msg.W, msg.H)
		return m, nil
	default:
		var cmd tea.Cmd
		m.dlg, cmd = m.dlg.Update(msg)
		return m, wrap(m.tab, DrainKind, cmd)
	}
}

// open は待機の対象を差し替え、必要なら計時とスピナを動かす。
func (m drainModal) open(msg drainOpenMsg) (tea.Model, tea.Cmd) {
	m.target, m.label = msg.runner, msg.label
	m.dlg.SetInput(drainInput(msg.runner))
	if m.started {
		return m, nil
	}
	m.started = true
	return m, wrap(m.tab, DrainKind, m.dlg.Start())
}

// refresh は待機中の対象を最新の検出結果で引き直す。
//
// **svc.Drain の progress コールバックでは更新しない。** コールバックは svc 側の
// goroutine から呼ばれ、そこから tea.Model を触ると競合する（-race で落ちる）。
// 3 秒ごとに届く共有状態から引き直すのは、詳細画面が「開いている間も一覧と同じ
// 周期で内容が更新される」のと同じ規則である（screens.md の詳細画面）。
//
// 検出から消えた runner は最後に見た姿のまま残す。空にすると、待機中に一時的な
// 検出漏れが起きたときだけ画面から対象が消え、何を待っているのか分からなくなる。
func (m *drainModal) refresh(st page.StateMsg) {
	for _, r := range st.Result.Runners {
		if r.Dir == m.target.Dir {
			m.target = r
			break
		}
	}
	m.dlg.SetInput(drainInput(m.target))
}

// View は待機の中身を返す。見出しは枠（template.Modal）が描く。
func (m drainModal) View() tea.View { return tea.NewView(m.dlg.View()) }

// drainInput は runner から待機画面の表示内容を組む。
//
// リポジトリ名を空にするのは、Runner.Worker（/proc 由来）にジョブのリポジトリ情報が
// 無いためである（Jobs タブの jobView と同じ扱い。dialog 側が "-" として描く）。
func drainInput(r runner.Runner) dialog.DrainInput {
	jobs := make([]dialog.DrainJob, 0, len(r.Workers))
	for _, w := range r.Workers {
		jobs = append(jobs, dialog.DrainJob{Repository: "", PID: w.PID, Elapsed: w.Elapsed()})
	}
	return dialog.DrainInput{Runner: r.Name(), Jobs: jobs}
}

// drainTitle はモーダルの見出しを返す。
//
// 進捗（"(2/3)"）を添えるのはここである。**dialog.DrainWaiter.Title には持たせない。**
// 「何件中の何件目か」は待機画面ではなく、対象の並びを持つ制御部（drainRun）の
// 知識だからである。
func drainTitle(model tea.Model) string {
	m, ok := model.(drainModal)
	if !ok {
		return ""
	}
	if m.label == "" {
		return m.dlg.Title()
	}
	return m.dlg.Title() + titleGap + m.label
}

// drainHints はフッタに出すキーヒントを返す。
func drainHints(model tea.Model) []atom.Hint {
	m, ok := model.(drainModal)
	if !ok {
		return nil
	}
	return m.dlg.Hints()
}
