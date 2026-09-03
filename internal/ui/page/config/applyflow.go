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
	"github.com/ousiassllc/gsr-helper/internal/ui/page/configmodal"
)

// 書き込みの結果を受けてから反映（FR-39）までを集める。results.go から分けたのは
// 1 ファイル 300 行の上限に収めるためで、責務の境界ではない。

// onDone は書き込みと反映の結果を処理する。
//
// ファイルを書き換えた場合だけ反映方法の選択へ進む（FR-39）。ラベルと runner
// group は GitHub 側で即時に反映され、再起動が要らない。
func (m *Model) onDone(msg doneMsg) tea.Cmd {
	m.busy = false
	saved, note := m.rememberSelf(msg.err)

	if msg.err != nil {
		m.report = "失敗: " + msg.err.Error()
		return nil
	}
	m.report = msg.text + note
	m.refresh(organism.KeepCursor)

	if m.pending.FileBacked() && len(m.applyTargets()) > 0 {
		return tea.Batch(saved, configmodal.OpenApply(&m.overlay, applyChoices()))
	}
	m.pending = edit.Change{}

	return saved
}

// auditRestartNote は監査ログの出力先を変えたときに結果へ添える案内（Issue #132）。
//
// **audit_log だけは走行中に反映しない。** 親が配るのは開いたファイルハンドル
// （ui.Options.Audit）であり、追従させるには走行中にログを開き直すことになる。
// 差し替えの最中に走っている書き込みを取りこぼす・二重に開くという危険を、
// 出力先の変更という頻度の低い操作のために抱える理由が無い。**代わりに黙って
// 効かないままにはしない**——伝えないと、利用者から見れば「設定に書いたパスが
// 効かない」という Issue #128 と同じ体験になる。
const auditRestartNote = "（監査ログの出力先は再起動後に切り替わります）"

// rememberSelf は書き込めた自身の設定を以後の初期値・差分の基準にする（FR-42）。
//
// 併せて**親へ返す Cmd を戻す**（Issue #128）。親の設定は起動時に 1 度決まるだけ
// なので、返さないと設定ファイルだけが新しくなり、ディスク閾値の判定は再起動まで
// 古い値のままになる。書き込めなかったときは nil を返し、親は古い値を持ち続ける。
//
// 2 つ目の戻り値は結果に添える案内である（auditRestartNote）。**比べる相手は
// 起動時の値（startupAudit）であって前回の保存値ではない。** 走っているログの
// 出力先を決めるのは起動時の audit_log なので、前回の保存値と比べると、一度
// 変えてから元へ戻した保存で「再起動後に切り替わります」と出てしまう——再起動
// しても何も変わらないのに再起動を促す、事実として誤った案内である。逆に
// audit_log を変えた後で別の項目だけを保存すると、再起動待ちが残っているのに
// 案内が消える。**変えていないときに添えないのは**、毎回の保存に無関係な案内を
// 添えると読み飛ばされ、本当に再起動が要るときにも伝わらなくなるためである。
func (m *Model) rememberSelf(err error) (tea.Cmd, string) {
	if !m.savingSelf {
		return nil, ""
	}
	m.savingSelf = false
	if err != nil {
		return nil, ""
	}

	note := ""
	if m.savedSelf.AuditLog != m.startupAudit {
		note = auditRestartNote
	}
	m.conf, m.confSet = m.savedSelf, true

	return page.ConfigSaved(m.conf), note
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
