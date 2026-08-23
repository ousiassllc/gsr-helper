package runnerop

import (
	"context"
	"strconv"

	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/runner"
	"github.com/ousiassllc/gsr-helper/internal/svc"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/action"
)

// ドレイン停止の段取り（待機画面の開閉・順次実行・キャンセル）を集める。

// drainRun は進行中のドレイン停止。
//
// **ポインタで持つ。** tea.Model は値で受け渡され、Update のたびに写しができる。
// context.CancelFunc を値のフィールドに置くと写しごとに枝分かれし、esc を受けた
// 写しが「もう捨てられた写しの cancel」を呼ぶ経路ができる。そうなると待機は
// 止まらず、画面だけが閉じる。page.Overlay が変わりうる状態を *overlayState に
// まとめているのと同じ理由である（page/overlay.go の Overlay の doc）。
type drainRun struct {
	// seq は待機を始めるたびに増える通し番号。
	//
	// キャンセルしても svc.Drain は ctx.Err() を返して戻るため、待機を畳んだ後に
	// 結果が 1 件届く。番号が合わない結果は捨てる。捨てないと、キャンセル直後に
	// 別の待機を始めた場合に前の待機の結果でそちらを進めてしまう。
	seq     int
	targets []runner.Runner
	index   int
	cancel  context.CancelFunc
	results []Result
}

// drainStepMsg は 1 件のドレイン停止が終わったことを表す。
type drainStepMsg struct {
	seq    int
	index  int
	runner string
	err    error
}

// drainOpenMsg は待機画面を開く指示。
type drainOpenMsg struct {
	runner runner.Runner
	// label は見出しに添える進捗（"(2/3)"）。対象が 1 件なら空。
	label string
}

// drainStopMsg は待機画面の計時とスピナを止める指示。
type drainStopMsg struct{}

// startDrain はドレイン停止を始める。
//
// **確認ダイアログを経ない。** 待機の開始にすぎず、待機中はいつでもキャンセル
// できるためである（screens.md の Runners タブの操作、Issue #5 の受け入れ条件）。
// 確認を挟むと、安全側の操作にだけ余分な打鍵が増えて `d` が使われなくなり、
// 結果として `x` や `X` でジョブを中断する運用に寄る。
func (m *Model) startDrain(targets []runner.Runner) tea.Cmd {
	m.seq++
	m.drain = &drainRun{seq: m.seq, targets: targets, index: 0, cancel: nil, results: nil}
	return m.openDrain()
}

// openDrain は現在の対象の待機画面を開き、ドレイン停止の Cmd を発行する。
//
// **対象が複数のときは順に処理する。** 同時に走らせると、待機画面がどの runner を
// 待っているのかを表せず（画面は 1 枚である）、停止の順序も制御できない。
//
// 待ち時間は無制限である（FR-07）。打ち切りの締切を ctx に入れてはならない。
// 中断は esc（cancelDrain）だけで行う。
func (m *Model) openDrain() tea.Cmd {
	d := m.drain
	r := d.targets[d.index]

	ctx, cancel := context.WithCancel(context.Background())
	d.cancel = cancel

	open := m.overlay.Open(DrainKind, drainOpenMsg{runner: r, label: progressLabel(d.index, len(d.targets))})

	ex := m.st.Exec
	seq, index := d.seq, d.index
	// progress は渡さない。コールバックは svc 側の goroutine から呼ばれ、そこから
	// tea.Model を触ると競合する。待機中の表示は 3 秒ごとに届く共有状態から
	// 引き直す（drainModal.refresh の doc）。
	run := page.Do(m.tab, func() tea.Msg {
		err := svc.Drain(ctx, ex, r, nil)
		cancel()
		return Msg{Payload: drainStepMsg{seq: seq, index: index, runner: r.Name(), err: err}}
	})
	return tea.Batch(open, run)
}

// stepDrain は 1 件ぶんの結果を取り込み、残りがあれば次へ進む。
func (m *Model) stepDrain(step drainStepMsg) tea.Cmd {
	d := m.drain
	// 畳んだ後・別の待機を始めた後に届いた古い結果は捨てる（drainRun.seq の doc）。
	if d == nil || d.seq != step.seq || d.index != step.index {
		return nil
	}

	d.results = append(d.results, Result{Runner: step.runner, Err: step.err})
	d.index++
	if d.index < len(d.targets) {
		return m.openDrain()
	}
	return m.finishDrain()
}

// cancelDrain は待機を取り消す。**残りの対象もすべて中止する。**
//
// 1 件だけ飛ばして次へ進む形にすると、esc を押したのに別の runner の待機が
// 始まってしまい、「取り消した」という利用者の意図と食い違う。
func (m *Model) cancelDrain() tea.Cmd {
	if m.drain == nil {
		return nil
	}
	if m.drain.cancel != nil {
		m.drain.cancel()
	}
	m.note.canceled = true
	return m.finishDrain()
}

// finishDrain は待機画面を閉じ、結果を報告する。
//
// 計時とスピナを止める指示は閉じた後にも届ける必要があるため page.ModalMsg で
// 宛先を明示する（page.Overlay.Handles は ModalMsg を常に引き受ける）。止め忘れると
// 待機を終えた後もスピナの Tick が流れ続け、他の画面の再描画を無駄に起こす
// （dialog.DrainWaiter.Stop の doc）。
func (m *Model) finishDrain() tea.Cmd {
	d := m.drain
	m.drain = nil
	m.overlay.Close()

	done := Msg{Payload: DoneMsg{Op: action.Drain, Results: d.results}}
	return tea.Batch(
		page.Do(m.tab, func() tea.Msg { return page.ModalMsg{Kind: DrainKind, Msg: drainStopMsg{}} }),
		page.Do(m.tab, func() tea.Msg { return done }),
	)
}

// progressLabel は見出しに添える進捗を返す。対象が 1 件なら空文字を返す。
//
// 1 件のときに "(1/1)" を出さないのは、分母のある表示が「順に処理している」ことの
// 合図だからである。1 件しか無いのに出すと、待っている対象が他にもあると読める。
func progressLabel(index, total int) string {
	if total <= 1 {
		return ""
	}
	return "(" + strconv.Itoa(index+1) + "/" + strconv.Itoa(total) + ")"
}
