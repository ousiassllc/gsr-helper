package setup

import (
	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/setup"
	"github.com/ousiassllc/gsr-helper/internal/ui/organism/dialog"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/progressmodal"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/setup/report"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/setupmodal"
)

// onFormResult は追加フォームの完了・中断を処理する。
func (m *Model) onFormResult(msg tea.Msg) tea.Cmd {
	switch msg.(type) {
	case dialog.FormDoneMsg:
		m.formShown = false
		m.overlay.Close()
		return m.planAdd()
	case dialog.FormAbortedMsg:
		m.formShown = false
		m.overlay.Close()
		return nil
	case dialog.FormDiscardMsg:
		// 入力済みのまま esc を押した。破棄してよいかを確認する
		// （atomic-design.md「Form と huh」）。**Form 自身は確認を出さない。**
		// モーダルを重ねられるのは Overlay を持つ page だけである。
		return setupmodal.OpenDiscard(&m.overlay)
	default:
		return nil
	}
}

// planAdd はフォームの入力から追加の計画を組み始める。
func (m *Model) planAdd() tea.Cmd {
	spec, err := m.vals.spec(m.st)
	if err != nil {
		m.notice = firstLine(err.Error())
		return nil
	}

	m.seq++
	m.waiting = true

	return m.planCmd(m.seq, spec.Scope, func(version string) (setup.Plan, error) {
		spec.Version = version
		return setup.PlanAdd(spec)
	})
}

// onProgressResult は進捗表示からの決定を処理する。
//
// 実行が終わったあとの esc は Overlay が閉じるため、ここへは届かない。
func (m *Model) onProgressResult(tea.Msg) tea.Cmd { return nil }

// onDone は実行の完了を処理する（FR-15 の結果報告）。
func (m *Model) onDone(msg doneMsg) tea.Cmd {
	if m.run == nil || msg.seq != m.run.seq {
		return nil
	}

	m.run.cancel()
	m.report = report.Lines(msg.result, msg.err)
	m.run.title = "完了"
	cmd := m.updateProgress()
	m.run = nil

	// スピナを止める。止め忘れると処理を終えた後も Msg が流れ続ける。
	stop := progressmodal.Stop(&m.overlay)
	return tea.Batch(cmd, stop)
}
