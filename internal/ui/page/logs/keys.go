package logs

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	dlogs "github.com/ousiassllc/gsr-helper/internal/logs"
	"github.com/ousiassllc/gsr-helper/internal/runner"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
)

// キー入力の解釈と、page の寿命（表裏・終了）への応答を集める。

// handleKey はキー入力を解釈する。
//
// 入力中とモーダル表示中は**親へ差し戻さない**。グローバルキーを閉じ込められるのは
// この判定を持つ page だけである（page.GlobalKeyMsg の doc）。
//
// **`tab` も差し戻さない。** この画面では `tab` がペインの切り替えであり、差し戻すと
// 親が「次のタブ」として解釈して 1 打鍵で 2 つの操作が起きる（keymap.LogKeys.Pane）。
func (m Model) handleKey(press tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	keys := m.st.Keys
	switch {
	case m.overlay.Active():
		return m.forward(press)
	case m.body.Filtering():
		return m.handleFilterKey(press)
	case key.Matches(press, keys.Global.Help):
		cmd = m.overlay.OpenHelp()
	case key.Matches(press, keys.Log.Pane):
		m.togglePane()
	case key.Matches(press, keys.Log.Follow):
		m.body.SetFollow(!m.body.Following())
	case key.Matches(press, keys.Log.Journal):
		cmd = m.toggleJournal()
	case key.Matches(press, keys.List.Filter):
		cmd = m.body.StartFilter()
	case m.focus == focusBody && key.Matches(press, keys.List.Bottom):
		// 末尾へ移動して追従を再開する（screens.md の Logs タブの `G`）。
		m.body.SetFollow(true)
	case m.focus == focusList && key.Matches(press, keys.List.Enter):
		cmd = m.openSelected()
	case key.Matches(press, keys.Global.Back):
		m.body.ClearFilter()
		m.applyLines()
	default:
		// 自分が解釈しないキーは操作中のペインへ渡し、同時に親へ差し戻す。
		next, c := m.forward(press)
		return next, tea.Batch(c, page.BubbleKey(press))
	}
	return m, tea.Batch(m.chrome(), cmd)
}

// handleFilterKey はフィルタの入力中のキーを処理する。
//
// 確定と取消だけを解釈し、残りは入力欄へ渡す（screens.md の入力中）。確定・取消の
// どちらでも行を絞り直すのは、入力の途中経過を本文へ反映していないためである。
func (m Model) handleFilterKey(press tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	keys := m.st.Keys.List
	switch {
	case key.Matches(press, keys.Accept):
		m.body.AcceptFilter()
	case key.Matches(press, keys.Cancel):
		m.body.CancelFilter()
	default:
		return m.forward(press)
	}
	m.applyLines()
	return m, m.chrome()
}

// togglePane は操作するペインを移す。
func (m *Model) togglePane() {
	if m.focus == focusList {
		m.focus = focusBody
		return
	}
	m.focus = focusList
}

// openSelected はカーソル位置のログを本文に開く。
func (m *Model) openSelected() tea.Cmd {
	cur, ok := m.tbl.Selected()
	if !ok {
		return nil
	}
	return m.open(target{runner: cur.Runner, file: cur.File, journal: false})
}

// toggleJournal は本文の表示元をログファイルと `journalctl` の間で切り替える（FR-26）。
//
// 使えない場合は何もしない。可否と理由はフッタに出しているので、押しても何も
// 起きないキーにはならない（screens.md の「無効な操作の表示」）。
func (m *Model) toggleJournal() tea.Cmd {
	if m.target.journal {
		return m.open(target{runner: m.target.runner, file: m.target.file, journal: false})
	}
	if _, ok := m.journalAllowed(); !ok {
		return nil
	}
	return m.open(target{runner: m.target.runner, file: m.target.file, journal: true})
}

// journalAllowed は `journalctl` へ切り替えられるかと、できない理由を返す。
func (m Model) journalAllowed() (reason string, ok bool) {
	switch {
	case !m.st.Caps.Journal:
		return reasonNoJournal, false
	case m.target.empty():
		return reasonNoTarget, false
	case m.target.runner.UnitName == "":
		return reasonNoUnit, false
	default:
		return "", true
	}
}

// showLatestWorker は runner の直近ジョブの Worker ログを開く（screens.md の `l`）。
//
// Runners / Jobs タブから親経由で届く（page.OpenTabMsg）。Worker ログが 1 つも無い
// 場合は対象を変えず、理由を状態行に出す。切り替えた先が空になるより、今の本文を
// 保ったまま理由を出すほうが、押した結果が読み取れる。
func (m Model) showLatestWorker(r runner.Runner) (tea.Model, tea.Cmd) {
	f, ok := dlogs.LatestWorker(r)
	if !ok {
		m.err = errNoWorkerLog(r.Name())
		return m, m.chrome()
	}
	m.focus = focusBody
	cmd := m.open(target{runner: r, file: f, journal: false})
	return m, tea.Batch(m.chrome(), cmd)
}

// activate は前面に戻ったことを受けて購読を張り直し、一覧を取り直す。
//
// **張り直す前に、持っている行を捨てる。** dlogs.Tail は購読のたびに末尾を読み直して
// 送出する（seekTail）ので、捨てないとタブを離れて戻るたびに同じ行が本文へ二重に
// 並ぶ。対象は変えないまま取り直した行で埋め直すので、裏へ回ったことが見えない
// （page.DeactivateMsg の doc）ままで二重取り込みだけが消える。
//
// 先に stop を通すのは、前の購読が畳まれないまま世代（stream.gen）だけ進むのを
// 避けるためである。err も落とす。追従し直す以上、前回の追従が失敗した理由を状態行に
// 残し続けると、今の状態を誤って伝える。
//
// open と同じ前処理だが束ねていない。open は対象の差し替えと追従の再開（SetFollow）も
// 行い、ここでは**どちらもしない**のが要点なので、共通化すると差が読めなくなる。
func (m Model) activate() (tea.Model, tea.Cmd) {
	m.active = true
	m.stop()
	m.lines = nil
	m.err = nil
	m.applyLines()
	sub := m.subscribe()
	return m, tea.Batch(m.chrome(), sub, m.listFiles())
}

// deactivate は裏へ回ったことを受けて購読を畳む。
//
// **状態は捨てない。** 対象・行・スクロール位置を残すのは、戻ったときに裏へ回った
// ことが見えないようにするためである（page.DeactivateMsg の doc）。
//
// 後始末の Cmd は返さない。畳むのは context のキャンセルだけで、Update の中で
// 済ませている（stream.stop の doc）。
func (m Model) deactivate() (tea.Model, tea.Cmd) {
	m.active = false
	m.stop()
	return m, nil
}

// shutdown は終了を受けて購読を畳む。裏に居ても届くので、畳み損ねを確実に閉じる。
func (m Model) shutdown() (tea.Model, tea.Cmd) {
	m.active = false
	m.stop()
	return m, nil
}
