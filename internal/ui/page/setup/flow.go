package setup

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/runner"
	"github.com/ousiassllc/gsr-helper/internal/runner/scope"
	"github.com/ousiassllc/gsr-helper/internal/setup"
	"github.com/ousiassllc/gsr-helper/internal/setup/job"
	"github.com/ousiassllc/gsr-helper/internal/ui/molecule"
	"github.com/ousiassllc/gsr-helper/internal/ui/organism/dialog"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/progressmodal"
)

// onRequest は他のタブ（Runners の n / D / u）からの依頼を処理する。
func (m *Model) onRequest(msg page.SetupRequestMsg) tea.Cmd {
	if m.run != nil {
		m.notice = noticeRunning
		return nil
	}

	switch msg.Op {
	case page.SetupAdd:
		return m.startAdd(bulkForm)
	case page.SetupRemove:
		return m.startRemove(msg.Runners)
	case page.SetupUpdate:
		return m.startUpdate(msg.Runners)
	default:
		return nil
	}
}

// startAdd は追加のフォームを開く。
func (m *Model) startAdd(kind formKindOf) tea.Cmd {
	if ok, why := m.allowed(page.BindingKey(m.st.Keys.Runner.Add)); !ok {
		m.notice = why
		return nil
	}

	m.pending = page.SetupAdd
	m.targets = nil
	m.vals.reset(m.st, kind)
	m.formShown = true

	return m.overlay.Open(formKind, formOpenMsg{kind: kind, values: m.vals, st: m.st})
}

// startRemove は削除の計画を組み始める。
func (m *Model) startRemove(targets []runner.Runner) tea.Cmd {
	if ok, why := m.allowedFor(m.st.Keys.Runner.Delete, targets); !ok {
		m.notice = why
		return nil
	}

	m.pending = page.SetupRemove
	m.targets = targets
	m.seq++
	m.waiting = true

	seq := m.seq
	spec := setup.RemoveSpec{Runners: targets}
	// 削除は API を引かずに計画が組める（バージョンを使わない）。それでも同じ
	// 経路を通すのは、計画の組み立てと承認の流れを 1 本に保つためである。
	return page.Do(m.tab, func() tea.Msg {
		p, err := setup.PlanRemove(spec)
		return planMsg{seq: seq, plan: p, scope: scopeOf(targets), err: err}
	})
}

// startUpdate はバージョン更新の計画を組み始める。
func (m *Model) startUpdate(targets []runner.Runner) tea.Cmd {
	if ok, why := m.allowedFor(m.st.Keys.Runner.Update, targets); !ok {
		m.notice = why
		return nil
	}
	if len(targets) == 0 {
		m.notice = noticeNoRunner
		return nil
	}

	m.pending = page.SetupUpdate
	m.targets = targets
	m.seq++
	m.waiting = true

	return m.planCmd(m.seq, scopeOf(targets), func(version string) (setup.Plan, error) {
		return setup.PlanUpdate(setup.UpdateSpec{Runners: targets, Version: version})
	})
}

// allowedFor は対象を伴う操作の可否を返す。対象ごとに判定し、1 台でも不可なら
// 理由を返す（ジョブ実行中の削除など。screens.md「無効な操作の表示」）。
func (m Model) allowedFor(b key.Binding, targets []runner.Runner) (bool, string) {
	k := page.BindingKey(b)
	if len(targets) == 0 {
		return m.allowed(k)
	}
	for _, r := range targets {
		if ok, why := m.actions.Allowed(k, r, m.st.Caps); !ok {
			return false, why
		}
	}
	return true, ""
}

// scopeOf は対象から API に使うスコープを選ぶ。
//
// tarball の取得情報はどのスコープから引いても同じ内容が返るため、先頭の 1 つで
// よい。短命トークンは台ごとのスコープで取り直す（token.go の tokenCache）。
func scopeOf(targets []runner.Runner) scope.Scope {
	if len(targets) == 0 {
		return scope.Scope{Kind: scope.Unknown, Owner: "", Repo: ""}
	}
	return targets[0].Scope
}

// onPlan は組み上がった計画を受けて実行前プレビューを開く。
func (m *Model) onPlan(msg planMsg) tea.Cmd {
	if msg.seq != m.seq {
		return nil
	}
	m.waiting = false

	if msg.err != nil {
		m.notice = firstLine(msg.err.Error())
		return nil
	}

	m.plan = msg.plan
	m.apiScope = msg.scope

	return m.overlay.Open(confirmKind, confirmOpenMsg{input: confirmInput(msg.plan)})
}

// onResult はモーダルの決定を処理する。
func (m *Model) onResult(msg page.ResultMsg) tea.Cmd {
	switch msg.Kind {
	case formKind:
		return m.onFormResult(msg.Msg)
	case confirmKind:
		decided, ok := msg.Msg.(dialog.DecidedMsg)
		if !ok {
			return nil
		}
		return m.confirmed(decided.Confirmed)
	case discardKind:
		decided, ok := msg.Msg.(dialog.DecidedMsg)
		if !ok {
			return nil
		}
		m.discarded(decided.Confirmed)
		return nil
	case progressmodal.Kind:
		return m.onProgressResult(msg.Msg)
	default:
		return nil
	}
}

// confirmed は実行前プレビューの承認・キャンセルを処理する。
//
// **setup.Apply へ至る経路はここだけである。** 承認以外から実行を始められる道を
// 作らないことで「確認を経ない破壊的経路を作らない」を構造として守る。
func (m *Model) confirmed(ok bool) tea.Cmd {
	plan := m.plan
	sc := m.apiScope
	m.plan = setup.Plan{}
	m.overlay.Close()

	if !ok || len(plan.Units) == 0 {
		return nil
	}
	return m.startRun(plan, sc)
}

// discarded は入力の破棄の可否を処理する。
//
// 破棄するなら確認とフォームの 2 枚を閉じてメニューへ戻る。やめるなら確認だけを
// 閉じ、入力途中のフォームへ戻す（Overlay.Close は最上位を 1 枚だけ閉じる）。
func (m *Model) discarded(ok bool) {
	m.overlay.Close()
	if !ok {
		return
	}
	m.overlay.Close()
	m.formShown = false
}

// onProgress は進捗を 1 件取り込み、次の 1 件を待つ。
func (m *Model) onProgress(msg progressMsg) tea.Cmd {
	if m.run == nil || msg.seq != m.run.seq {
		return nil
	}
	if !msg.ok {
		return nil
	}

	m.run.apply(msg.progress)
	return tea.Batch(m.updateProgress(), m.waitProgress(msg.seq, m.run.ch))
}

// apply は進捗 1 件を行の状態へ反映する。
func (r *runState) apply(p setup.Progress) {
	if p.Index == job.PrepIndex {
		// 準備段階は台に紐付かないので件数を添えない（1/3 の数え方が無い）。
		r.title, r.bare = p.Phase+"中…", p.Phase+"中…"
		return
	}
	if p.Index < 0 || p.Index >= len(r.rows) {
		return
	}

	switch {
	case p.Err != nil:
		r.rows[p.Index].State = molecule.ProgressFailed
		r.rows[p.Index].Detail = "失敗（" + p.Phase + "）"
	case p.Done:
		r.rows[p.Index].State = molecule.ProgressDone
		r.rows[p.Index].Detail = "完了"
		r.done = p.Index + 1
	default:
		r.rows[p.Index].State = molecule.ProgressRunning
		r.rows[p.Index].Detail = p.Phase + "…"
	}
	r.title, r.bare = runTitle(r.kind, r.done, r.total), bareTitle(r.kind)
}

// waitingRows は計画から「待機」状態の行を作る。
func waitingRows(plan setup.Plan) []molecule.ProgressView {
	out := make([]molecule.ProgressView, 0, len(plan.Units))
	for _, u := range plan.Units {
		out = append(out, molecule.ProgressView{
			Name: u.Name, State: molecule.ProgressWaiting, Detail: "待機",
		})
	}
	return out
}
