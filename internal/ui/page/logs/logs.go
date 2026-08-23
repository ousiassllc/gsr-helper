// Package logs は Logs タブを実装する（FR-23〜FR-26）。
//
// ホスト上の runner の `_diag` 配下のログを 1 つの一覧に並べ、選んだログを追従表示する。
// systemd ユニットのログ（`journalctl`）も同じ本文のペインで扱い、`J` で切り替える。
//
// **ペインは上下に分ける。** screens.md は「ファイル一覧と本文の 2 ペイン」とだけ定めて
// いるが、保証する端末幅は 80 であり（token.WidthTarget）、左右に割ると本文がログ 1 行を
// 出せる幅にならない。上に一覧、下に本文を置き、`tab` で操作するペインを移す。
//
// ドメイン層の logs はこのパッケージと同名なので dlogs として import する
// （organism/table が bubbles/table を btable とするのと同じ扱い）。
package logs

import (
	"regexp"

	tea "charm.land/bubbletea/v2"

	dlogs "github.com/ousiassllc/gsr-helper/internal/logs"
	"github.com/ousiassllc/gsr-helper/internal/ui/organism/pane"
	"github.com/ousiassllc/gsr-helper/internal/ui/organism/table"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
)

// focus はキーを受けるペイン。
type focus int

const (
	// focusList はファイル一覧のペイン。
	focusList focus = iota
	// focusBody は本文のペイン。
	focusBody
)

// Model は Logs タブ。
type Model struct {
	tab int
	st  page.StateMsg

	tbl     table.Model[row]
	body    pane.Log
	overlay page.Overlay

	focus focus
	// target は今追っている対象。空なら本文に案内を出す。
	target target
	// lines は購読から届いた行。古いものから捨てて maxLines 行に保つ。
	lines []dlogs.Line
	// err は追従の失敗。次に対象を切り替えるまで状態行に出す。
	err error
	// filterErr はフィルタが正規表現として解けなかった理由。
	//
	// err と分けているのは、直せるのが利用者（打ち直す）か環境（ログが読めない）かで
	// 意味が違うためである。どちらも状態行に出すが、フィルタの誤りを先に出す。
	filterErr error
	// styled は絞り込み・装飾を通した行。本文へ渡しているものと同じ並びで、行が 1 行届く
	// たびに全行を作り直さないために持つ（content.go の doc）。
	styled []string
	// filterRe / filterSrc は解いた正規表現と、その元になったフィルタ文字列。文字列を添えて
	// 持つのは、フィルタが変わったときだけ解き直すためである（content.go の filterRegexp）。
	filterRe  *regexp.Regexp
	filterSrc string
	// stream は購読 1 本ぶんの世代と停止手段。
	stream stream
	// active は前面に居るか。裏で `_diag` を列挙し直さないための判定である。
	active bool

	// initCmd はモーダルを登録したときに返った Cmd。最初の共有状態で流し、nil に落とす。
	initCmd tea.Cmd
}

// tea.Model を実装していることをコンパイル時に確かめる。
var _ tea.Model = Model{}

// New は Logs タブを組み立てる。tab は親が持つタブ番号で、ChromeMsg に載せる。
//
// ? の範囲を差し替えるのは、Logs タブのキー集合が runner を並べる一覧と違うためで
// ある（keymap.Set.LogsHelp）。既定のままだと runner の操作キーがヘルプに並ぶ。
func New(tab int, st page.StateMsg) Model {
	overlay, help := page.NewOverlay(tab, st)
	scope := overlay.SetHelpScope(logsHelp)
	return Model{
		tab:       tab,
		st:        st,
		tbl:       newTable(st.Keys, st.Styles),
		body:      pane.NewLog(st.Styles),
		overlay:   overlay,
		focus:     focusList,
		target:    target{},
		lines:     nil,
		err:       nil,
		filterErr: nil,
		styled:    nil,
		filterRe:  nil,
		filterSrc: "",
		stream:    stream{},
		active:    false,
		initCmd:   tea.Batch(help, scope),
	}
}

// Init は何も発行しない。親はタブの Init を呼ばないため、登録が返した Cmd は
// 最初の page.StateMsg で流す（runners.go の Init と同じ理由）。
func (m Model) Init() tea.Cmd { return nil }

// Update は共有状態の反映・キー入力・購読からの行を処理する。
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case page.StateMsg:
		return m.setState(msg)
	case tea.KeyPressMsg:
		return m.handleKey(msg)
	case page.ActivateMsg:
		return m.activate()
	case page.DeactivateMsg:
		return m.deactivate()
	case page.ShutdownMsg:
		return m.shutdown()
	case page.ShowLogMsg:
		return m.showLatestWorker(msg.Runner)
	case page.ResultMsg:
		// モーダルが返した決定は page が受ける（runners.go と同じ理由）。この版の
		// Logs タブはモーダルからの決定を持たない（開くのはヘルプだけ）。
		return m, m.chrome()
	case filesMsg:
		return m.setFiles(msg)
	case lineMsg:
		return m.addLine(msg)
	case endMsg:
		return m.endStream(msg)
	default:
		return m.forward(msg)
	}
}

// View は一覧・見出し・本文を縦に並べて返す。
func (m Model) View() tea.View {
	if m.overlay.Active() {
		return tea.NewView(m.overlay.View())
	}
	return tea.NewView(m.render())
}

// setState は共有状態のスナップショットを反映する。
//
// 前面に居る間は `_diag` を列挙し直す Cmd を返す。ログファイルは実行中に増える
// （ジョブごとに Worker ログが 1 つ増える）ため、一覧を配られた検出結果だけから
// 作れない（FR-24 の「ファイル追加を検知して更新する」）。裏では列挙しない。
// 見ていない一覧のために 3 秒ごとに runner 台数ぶんの readdir を出す理由が無い。
func (m Model) setState(st page.StateMsg) (tea.Model, tea.Cmd) {
	m.st = st
	m.tbl.Restyle(st.Keys.List, st.Styles)
	m.body.Restyle(st.Styles)
	m.resize()

	init := m.flushInit()
	over := m.overlay.SetState(st)

	var list tea.Cmd
	if m.active {
		list = m.listFiles()
	}
	return m, tea.Batch(m.chrome(), init, over, list)
}

// flushInit は登録が返した Cmd を 1 度だけ返す（runners.go と同じ理由）。
func (m *Model) flushInit() tea.Cmd {
	cmd := m.initCmd
	m.initCmd = nil
	return cmd
}

// forward はキー以外の Msg を配る。宛先の判定は page.Overlay.Handles に任せる。
//
// Overlay が受けないものは操作中のペインへ渡す。カーソルの点滅のように、organism が
// 発行し続ける Cmd を止めないためである。
func (m Model) forward(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	if m.overlay.Handles(msg) {
		m.overlay, cmd = m.overlay.Update(msg)
		return m, tea.Batch(m.chrome(), cmd)
	}
	if m.focus == focusList && !m.body.Filtering() {
		m.tbl, cmd = m.tbl.Update(msg)
		return m, tea.Batch(m.chrome(), cmd)
	}
	m.body, cmd = m.body.Update(msg)
	return m, tea.Batch(m.chrome(), cmd)
}
