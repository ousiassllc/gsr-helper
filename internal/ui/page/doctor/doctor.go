// Package doctor は Doctor タブを実装する（FR-32〜FR-34）。
//
// 各チェックを並列に実行し、OK / WARN / FAIL / SKIP で一覧表示する。FAIL / WARN の
// 項目は enter で詳細（検出内容・影響・推奨する対処）を開き、r で全項目を、詳細画面の
// r でその 1 項目だけを再実行できる。
//
// **対処は表示するだけで実行しない。** 診断の責務を超えるためであり、sudoers の
// 編集・パッケージの導入・usermod はいずれも本ツールから行わない
// （docs/architecture/security.md）。
//
// 検出（runner 一覧）は自分で行わず、親 Model から page.StateMsg で受け取った
// スナップショットを診断の入力に使う（atomic-design.md の page の責務）。
package doctor

import (
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/doctor"
	"github.com/ousiassllc/gsr-helper/internal/ui/keymap"
	"github.com/ousiassllc/gsr-helper/internal/ui/molecule"
	"github.com/ousiassllc/gsr-helper/internal/ui/organism/table"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
)

const (
	// inputFilter は入力中であることを状態行に出すときの名称（screens.md の入力中）。
	inputFilter = "絞り込み"
	// emptyMessage はまだ 1 度も診断していないときの表示。
	emptyMessage = "診断を実行しています…"
	// noResultMessage は診断したが 1 件も結果が無いときの表示。
	noResultMessage = "診断結果がありません"
	// detailDesc は enter の説明。
	detailDesc = "詳細"
	// rerunDesc は r の説明。
	rerunDesc = "全項目を再実行"
	// runningStatus は実行中に状態行へ出す文。
	runningStatus = "診断を実行中です…"
	// lastRunLayout は見出しに出す最終実行時刻の書式。
	//
	// 日付を出さないのは、診断が「今のホストの状態」を見るものであり、日をまたいで
	// 見比べる値ではないためである（screens.md の Doctor タブの見出し）。
	lastRunLayout = "15:04:05"
)

// Model は Doctor タブ。
type Model struct {
	tab     int
	st      page.StateMsg
	tbl     table.Model[row]
	overlay page.Overlay
	// results は直近の診断結果。整列済み（doctor.Sort）。
	results []doctor.CheckResult
	summary doctor.Summary
	lastRun time.Time
	// running は診断の実行中か。二重に走らせないための番人でもある。
	running bool
	// initCmd はモーダルを登録したときに返った Cmd。最初の共有状態で流し、nil に落とす。
	initCmd tea.Cmd
	// started は最初の診断を発行したか。タブを行き来するたびに走らせないために持つ。
	started bool
	// detail は詳細画面が今出している結果の指し先。個別再実行のあとに同じ行の
	// 新しい結果へ差し替えるために覚えておく（モーダルが抱えている結果を page から
	// 読む手段は無い。detail.go の detailKey）。
	detail detailKey
}

// tea.Model を実装していることをコンパイル時に確かめる。
var _ tea.Model = Model{}

// New は Doctor タブを組み立てる。tab は親が持つタブ番号で、ChromeMsg に載せる。
func New(tab int, st page.StateMsg) Model {
	overlay, help := page.NewOverlay(tab, st)
	detail := overlay.Register(Kind, newModal(st))
	// ? に出すキーの範囲を宣言する。既定は runner を並べる一覧向けなので、
	// この画面で効かない runner の操作キーが並んでしまう（page.Overlay の doc）。
	scope := overlay.SetHelpScope(func(k keymap.Set) [][]key.Binding { return k.DoctorHelp() })
	return Model{
		tab:     tab,
		st:      st,
		tbl:     newTable(st.Keys, st.Styles),
		overlay: overlay,
		results: nil,
		summary: doctor.Summary{},
		lastRun: time.Time{},
		running: false,
		initCmd: tea.Batch(help, detail, scope),
		started: false,
		detail:  detailKey{},
	}
}

// Init は何も発行しない。親はタブの Init を呼ばないため、登録が返した Cmd と
// 最初の診断は最初の page.StateMsg で流す（jobs.go の Init と同じ理由）。
func (m Model) Init() tea.Cmd { return nil }

// Update は共有状態の反映、キー入力、診断結果の取り込みを行う。
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case page.StateMsg:
		return m.setState(msg)
	case tea.KeyPressMsg:
		return m.handleKey(msg)
	case page.ActivateMsg:
		// **診断を始めるのはタブが前面に出たときである。** 共有状態は起動直後から
		// 裏のタブにも配られるので、最初の StateMsg で始めると、Doctor タブを一度も
		// 開かない利用者のホストでも起動のたびに到達性の確認と journalctl が走る。
		// 起動時に要る前提チェック（FR-43 の 4 点）は親が別に走らせる（FR-44）。
		return m.activate()
	case doneMsg:
		cmd := m.applyDone(msg)
		return m, tea.Batch(m.chrome(), cmd)
	case page.ResultMsg:
		// モーダルが返した決定は page が受ける（page.Overlay.Handles が
		// ResultMsg に偽を返すことと合わせた二重の守り）。
		return m.handleResult(msg)
	default:
		return m.forward(msg)
	}
}

// View はモーダルが開いていればそれを、無ければ見出しと一覧を返す。
func (m Model) View() tea.View {
	if m.overlay.Active() {
		return tea.NewView(m.overlay.View())
	}
	return tea.NewView(m.heading() + "\n\n" + m.body())
}

// heading は判定ごとの件数と最終実行時刻の行を返す（screens.md の Doctor タブ）。
func (m Model) heading() string {
	v := molecule.SummaryView{
		OK: m.summary.OK, Warn: m.summary.Warn,
		Fail: m.summary.Fail, Skip: m.summary.Skip,
	}
	var updated string
	if !m.lastRun.IsZero() {
		updated = m.lastRun.Format(lastRunLayout)
	}
	return molecule.SummaryLine(v, updated, m.st.BodyW, m.st.Styles)
}

// body は一覧を返す。行が無ければ理由を出す。
func (m Model) body() string {
	if out := m.tbl.View(); out != "" {
		return out
	}
	if m.lastRun.IsZero() {
		return m.st.Styles.Muted.Render(emptyMessage)
	}
	return m.st.Styles.Muted.Render(noResultMessage)
}

// setState は共有状態のスナップショットを反映する。ドメイン層は呼ばない。
//
// **ここで診断を始めない。** 共有状態は 3 秒ごとに全タブへ配られるので、
// 始めると診断が絶え間なく走る。開始の契機は page.ActivateMsg である（activate）。
func (m Model) setState(st page.StateMsg) (tea.Model, tea.Cmd) {
	m.st = st
	m.tbl.Restyle(st.Keys.List, st.Styles)
	// 見出し 1 行と空行 1 行を引いた残りが一覧の領域である。
	m.tbl.SetSize(st.BodyW, st.BodyH-headingLines)
	init := m.flushInit()
	cmd := m.overlay.SetState(st)
	return m, tea.Batch(m.chrome(), init, cmd)
}

// activate はタブが前面に出たときの処理を返す。
//
// 初回だけ診断を始める。2 回目以降も走らせると、タブを行き来するたびに
// `sudo -l -U` と `journalctl -k` が監査ログへ積み上がる。**結果は裏に回っても
// 捨てない**ので、戻ったときに前回の結果がそのまま出る（page.DeactivateMsg の
// 「状態そのものは捨てない」）。
func (m Model) activate() (tea.Model, tea.Cmd) {
	if m.started {
		return m, m.chrome()
	}
	m.started = true
	m.running = true
	return m, tea.Batch(m.chrome(), m.startAll())
}

// headingLines は見出しが使う行数（件数の行 + 空行）。
const headingLines = 2

// flushInit は登録が返した Cmd を 1 度だけ返す。
func (m *Model) flushInit() tea.Cmd {
	cmd := m.initCmd
	m.initCmd = nil
	return cmd
}

// forward はキー以外の Msg を配る。宛先の判定は page.Overlay.Handles に任せる。
func (m Model) forward(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	if m.overlay.Handles(msg) {
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
	var cmd tea.Cmd
	switch {
	case m.tbl.Filtering(), m.overlay.Active():
		return m.forward(press)
	case key.Matches(press, m.st.Keys.Global.Help):
		cmd = m.overlay.OpenHelp()
	case key.Matches(press, m.st.Keys.List.Enter):
		cmd = m.openDetail()
	case key.Matches(press, m.st.Keys.Global.Refresh):
		// r は「全項目の再実行」であると同時に親の「再読み込み」でもある
		// （Disk タブと同じ扱い。差し戻さないと Doctor タブに居る間だけ
		// runner の再検出が止まる）。
		next, c := m.rerunAll()
		return next, tea.Batch(next.chrome(), c, page.BubbleKey(press))
	case key.Matches(press, m.st.Keys.Global.Back):
		m.tbl.ClearFilter()
	default:
		next, c := m.forward(press)
		return next, tea.Batch(c, page.BubbleKey(press))
	}
	return m, tea.Batch(m.chrome(), cmd)
}

// rerunAll は全項目の再実行を始める。実行中なら重ねない。
func (m Model) rerunAll() (Model, tea.Cmd) {
	if m.running {
		return m, nil
	}
	m.running = true
	return m, m.startAll()
}

// openDetail はカーソル位置の項目の詳細を開く。
func (m *Model) openDetail() tea.Cmd {
	cur, ok := m.tbl.Selected()
	if !ok {
		return nil
	}
	m.detail = keyOf(cur.result)
	return openDetail(&m.overlay, cur.result)
}

// handleResult は詳細画面から返った決定を実行する。
//
// **Cmd を得てから実行中の印を立てる。** 先に立てると、レジストリに無い識別子
// （startOne が nil を返す場合）で「診断を実行中です…」が戻らなくなり、
// 以後の再実行までフッタの r が無効のままになる。
func (m Model) handleResult(msg page.ResultMsg) (tea.Model, tea.Cmd) {
	re, ok := msg.Msg.(RecheckMsg)
	if !ok || m.running {
		return m, m.chrome()
	}
	cmd := m.startOne(re.ID)
	if cmd == nil {
		return m, m.chrome()
	}
	m.running = true
	return m, tea.Batch(m.chrome(), cmd)
}
