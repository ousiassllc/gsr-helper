package config

import (
	"context"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/config/apply"
	"github.com/ousiassllc/gsr-helper/internal/config/edit"
	"github.com/ousiassllc/gsr-helper/internal/runner"
	"github.com/ousiassllc/gsr-helper/internal/ui/organism"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
)

// 書き込みの結果を受けてから反映（FR-39）までを集める。results.go から分けたのは
// 1 ファイル 300 行の上限に収めるためで、責務の境界ではない。

// onDone は書き込みと反映の結果を処理する。
//
// ファイルを書き換えた場合だけ反映方法の選択へ進む（FR-39）。ラベルと runner
// group は GitHub 側で即時に反映され、再起動が要らない。
func (m *Model) onDone(msg doneMsg) tea.Cmd {
	m.busy = false
	m.rememberSelf(msg.err)

	if msg.err != nil {
		m.report = "失敗: " + msg.err.Error()
		return nil
	}
	m.report = msg.text
	m.refresh(organism.KeepCursor)

	if m.pending.FileBacked() && len(m.applyTargets()) > 0 {
		return m.overlay.Open(applyKind, applyOpenMsg{items: applyChoices()})
	}
	m.pending = edit.Change{}

	return nil
}

// rememberSelf は書き込めた自身の設定を以後の初期値・差分の基準にする（FR-42）。
func (m *Model) rememberSelf(err error) {
	if !m.savingSelf {
		return
	}
	m.savingSelf = false
	if err == nil {
		m.conf, m.confSet = m.savedSelf, true
	}
}

// applyChoices は反映方法の選択肢を返す。既定（ドレイン再起動）を先頭に置く。
func applyChoices() []organism.Choice {
	ms := apply.Methods()
	out := make([]organism.Choice, 0, len(ms))

	for _, method := range ms {
		out = append(out, organism.Choice{
			ID: method.Label(), Key: "", Desc: method.Label(), Impact: "", Reason: "",
			Enabled: true, DividerBefore: false,
		})
	}
	return out
}

// applyTargets は反映（再起動）の対象を返す（Change.ApplyTargets の doc）。
func (m Model) applyTargets() []runner.Runner {
	return m.pending.ApplyTargets(m.st.Result.Runners, m.target)
}

// onApplyChosen は選ばれた反映方法を実行する。
func (m *Model) onApplyChosen(msg tea.Msg) tea.Cmd {
	chosen, ok := msg.(organism.ChosenMsg)
	m.overlay.Close()

	if !ok {
		m.pending = edit.Change{}
		return nil
	}

	in := apply.Input{
		Exec: m.st.Exec, Runner: runner.Runner{}, Method: apply.FromLabel(chosen.ID),
		Reload: m.pending.Reload(), Progress: nil, Drain: nil,
	}
	targets := m.applyTargets()
	m.pending = edit.Change{}
	m.busy = true

	return page.Do(m.tab, func() tea.Msg {
		done, err := apply.RunAll(context.Background(), in, targets)
		if err != nil {
			return doneMsg{text: "", err: err}
		}
		return doneMsg{text: "反映しました（" + strings.Join(done, ", ") + "）", err: nil}
	})
}
