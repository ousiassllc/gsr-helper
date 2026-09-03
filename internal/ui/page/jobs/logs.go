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
	if chosen, ok := msg.Msg.(runnerdetail.ChosenMsg); ok && chosen.Action == action.Logs {
		m.overlay.Close()
		return m, tea.Batch(m.chrome(), showLog(chosen.Runner))
	}
	// ログ以外の決定はサービス制御が解釈する。確認ダイアログと待機画面の決定も
	// ここを通るため、握り潰すと y を押しても何も実行されない。
	//
	// ops の呼び出しを return より前に出すのは jobs.go の runnerop.Msg と同じ理由
	// （chrome が ops の変更前の状態を読まないようにするため）。
	c := m.ops.Result(msg)
	return m, tea.Batch(m.chrome(), c)
}

// showLog は Logs タブへ移り、直近ジョブの Worker ログを開くよう親へ求める Cmd を返す。
func showLog(r runner.Runner) tea.Cmd {
	return page.OpenTab(page.TabLogs, page.ShowLogMsg{Runner: r})
}
