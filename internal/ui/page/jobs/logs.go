package jobs

import (
	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/runner"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/action"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/runnerdetail"
)

// `l`（ログを開く）の結線を集める。Runners タブと同じ 2 つの起点（一覧の直接キーと
// 詳細画面の操作リスト）を持つ。**フッタに出しているキーは必ず結線する。** 出したまま
// 結線しないと「押せるが何も起きない」経路になる（screens.md の設計原則 2）。

// openLogs はカーソル位置のジョブを実行している runner のログを Logs タブで開く。
//
// 操作対象がジョブではなく runner であるのは Jobs タブ共通の約束である（FR-47）。
// ジョブ単位のログはその runner の直近の Worker ログとして開く。
func (m Model) openLogs() tea.Cmd {
	cur, ok := m.tbl.Selected()
	if !ok {
		return nil
	}
	return showLog(cur.runner)
}

// handleResult は詳細画面が返した決定を処理する（runners.handleResult と同じ理由）。
func (m Model) handleResult(msg page.ResultMsg) (tea.Model, tea.Cmd) {
	chosen, ok := msg.Msg.(runnerdetail.ChosenMsg)
	if !ok || chosen.Action != action.Logs {
		return m, m.chrome()
	}
	m.overlay.Close()
	return m, tea.Batch(m.chrome(), showLog(chosen.Runner))
}

// showLog は Logs タブへ移り、直近ジョブの Worker ログを開くよう親へ求める Cmd を返す。
func showLog(r runner.Runner) tea.Cmd {
	return page.OpenTab(page.TabLogs, page.ShowLogMsg{Runner: r})
}
