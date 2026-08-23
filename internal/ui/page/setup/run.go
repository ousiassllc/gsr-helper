package setup

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/runner/scope"
	"github.com/ousiassllc/gsr-helper/internal/setup"
	"github.com/ousiassllc/gsr-helper/internal/setup/job"
	"github.com/ousiassllc/gsr-helper/internal/ui/organism/pane"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/progressmodal"
)

// apiTimeout は計画を組むための API 呼び出しの上限。
//
// 一覧の 3 秒ポーリングとは無関係に、利用者の操作を起点に 1 度だけ呼ぶ
// （docs/api/external-interfaces.md「レート制限とエラー」）。
const apiTimeout = 30 * time.Second

// planMsg は計画が組み上がったことを伝える。
type planMsg struct {
	seq   int
	plan  setup.Plan
	scope scope.Scope
	err   error
}

// progressMsg は実行の進捗 1 件。ok が偽なら進捗の打ち切り。
type progressMsg struct {
	seq      int
	progress setup.Progress
	ok       bool
}

// doneMsg は実行が終わったことを伝える。
type doneMsg struct {
	seq    int
	result setup.Result
	err    error
}

// deps は共有状態から外部資源への入口を組み立てる。
//
// Model の写しを goroutine へ持ち込まないよう、必要な値だけを取り出す。
func (m Model) deps() job.Deps {
	return job.Deps{
		Exec: m.st.Exec, Secrets: m.st.Setup.Secrets,
		// 差し替えの口はそのまま渡す。ここで nil に潰すと、テストが挿した
		// 偽物が効かず本物の GitHub を叩く（page.SetupDeps.NewClient）。
		NewClient: m.st.Setup.NewClient, Fetch: m.st.Setup.Fetch,
	}
}

// planCmd は最新バージョンを引いてから計画を組む Cmd を返す。
func (m Model) planCmd(
	seq int,
	sc scope.Scope,
	build func(version string) (setup.Plan, error),
) tea.Cmd {
	d := m.deps()
	return page.Do(m.tab, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), apiTimeout)
		defer cancel()

		version, err := job.LatestVersion(ctx, d)
		if err != nil {
			return planMsg{seq: seq, plan: setup.Plan{}, scope: sc, err: err}
		}

		p, err := build(version)
		return planMsg{seq: seq, plan: p, scope: sc, err: err}
	})
}

// startRun は承認済みの計画を実行し、進捗を流し始める。
func (m *Model) startRun(plan setup.Plan, sc scope.Scope) tea.Cmd {
	m.seq++
	seq := m.seq

	in := job.Input{
		Deps: m.deps(), Plan: plan, Scope: sc, Drain: nil, Progress: nil,
	}

	ctx, cancel := context.WithCancel(context.Background())
	// 手順は 1 台あたり最大 5 本 + 準備と完了。UI が遅れても worker を止めない
	// よう、詰まらない大きさの緩衝を取る。
	ch := make(chan setup.Progress, len(plan.Units)*6+2)
	doneCh := make(chan doneMsg, 1)

	in.Progress = func(p setup.Progress) { ch <- p }

	go func() {
		res, err := job.Run(ctx, in)
		close(ch)
		doneCh <- doneMsg{seq: seq, result: res, err: err}
	}()

	kind := plan.Kind.String()
	m.run = &runState{
		seq: seq, cancel: cancel, ch: ch,
		rows: waitingRows(plan), done: 0, total: len(plan.Units),
		title: runTitle(kind, 0, len(plan.Units)), bare: bareTitle(kind), kind: kind,
	}
	m.report = nil

	return tea.Batch(m.waitProgress(seq, ch), m.waitDone(doneCh), m.openProgress(plan))
}

// waitProgress は進捗を 1 件受け取る Cmd を返す。
func (m Model) waitProgress(seq int, ch <-chan setup.Progress) tea.Cmd {
	return page.Do(m.tab, func() tea.Msg {
		p, ok := <-ch
		return progressMsg{seq: seq, progress: p, ok: ok}
	})
}

// waitDone は完了を受け取る Cmd を返す。
func (m Model) waitDone(ch <-chan doneMsg) tea.Cmd {
	return page.Do(m.tab, func() tea.Msg { return <-ch })
}

// openProgress は進捗表示を開く。見出しに件数を含めないのは、pane.ProgressList
// が Done/Total から自分で添えるためである（chrome.go の bareTitle）。
func (m Model) openProgress(plan setup.Plan) tea.Cmd {
	return progressmodal.Open(&m.overlay, pane.ProgressInput{
		Title:  bareTitle(plan.Kind.String()),
		Rows:   waitingRows(plan),
		Done:   0,
		Total:  len(plan.Units),
		Report: nil,
	})
}

// updateProgress は進捗表示へ現在の状態を送る。
func (m Model) updateProgress() tea.Cmd {
	if m.run == nil {
		return nil
	}

	return progressmodal.Set(&m.overlay, pane.ProgressInput{
		Title:  m.run.bare,
		Rows:   m.run.rows,
		Done:   m.run.done,
		Total:  m.run.total,
		Report: m.report,
	})
}
