package setup

import (
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/setup"
	"github.com/ousiassllc/gsr-helper/internal/ui/organism/dialog"
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
		// 入力済みの中断は破棄の確認を経る。確認は dialog.Form が Confirm を
		// 重ねて出すため、page 側では何もしない（atomic-design.md「Form と huh」）。
		return nil
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
	m.report = reportLines(msg.result, msg.err)
	m.run.title = "完了"
	cmd := m.updateProgress()
	m.run = nil

	// スピナを止める。止め忘れると処理を終えた後も Msg が流れ続ける。
	stop := m.overlay.Open(progressKind, progressStopMsg{})
	return tea.Batch(cmd, stop)
}

// reportLines は結果報告の行を組み立てる。
//
// 何台目までが成功し、どこで何が失敗したかを示す（FR-15）。成功分は残っている
// ことも明記する——失敗を見た利用者が、途中まで作った runner を手で消すべきか
// 判断できるようにするためである。
func reportLines(res setup.Result, err error) []string {
	if err == nil {
		return []string{"完了: " + strconv.Itoa(len(res.Succeeded)) + " 台"}
	}

	out := make([]string, 0, 4)
	if res.Failed != "" {
		out = append(out, "✗ "+res.Failed+" の"+res.Phase+"で失敗しました")
	}
	out = append(out, "  "+firstLine(err.Error()))

	// 並びは screens.md の進捗のモックに合わせる（完了 → 未実行 → 残る旨）。
	if n := len(res.Succeeded); n > 0 {
		out = append(out, "完了: "+strconv.Itoa(n)+" 台（"+strings.Join(res.Succeeded, ", ")+"）")
	}
	if n := len(res.Remaining); n > 0 {
		out = append(out, "未実行: "+strconv.Itoa(n)+" 台（"+strings.Join(res.Remaining, ", ")+"）")
	}
	if len(res.Succeeded) > 0 {
		out = append(out, strings.Join(res.Succeeded, ", ")+" はそのまま残っています。")
	}
	return out
}
