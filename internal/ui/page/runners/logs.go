package runners

import (
	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/runner"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/action"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/runnerdetail"
)

// `l`（ログを開く）の結線を集める。一覧の直接キーと詳細画面の操作リストの 2 つの
// 起点があり、どちらも同じ Cmd に落とす（screens.md の設計原則 6「操作の起点は複数」）。

// openLogs はカーソル位置の runner のログを Logs タブで開く Cmd を返す。
//
// 孤児ユニットの行では何もしない。対応する runner ディレクトリが無く、`_diag` を
// 持たないためである（openDetail と同じ理由）。
func (m Model) openLogs() tea.Cmd {
	cur, ok := m.tbl.Selected()
	if !ok || cur.isOrphan {
		return nil
	}
	return showLog(cur.runner)
}

// handleResult は詳細画面・確認ダイアログ・待機画面が返した決定を処理する。
//
// ログを開く決定だけをここで処理し、残りはサービス制御（runnerop）へ渡す。ログは
// 他のタブへ移る操作で、実行も確認も伴わないためこのタブの関心事である。
func (m Model) handleResult(msg page.ResultMsg) (tea.Model, tea.Cmd) {
	if chosen, ok := msg.Msg.(runnerdetail.ChosenMsg); ok && chosen.Action == action.Logs {
		// 詳細画面は閉じる。Logs タブへ移ったあとも開いたままだと、戻ってきたときに
		// 別の runner の詳細が残る。
		m.overlay.Close()
		return m, tea.Batch(m.chrome(), showLog(chosen.Runner))
	}
	// ops の呼び出しを return より前に出すのは、同じ tea.Batch に並べると chrome が
	// ops の変更前の状態を読み、キャンセルで閉じたダイアログが Modal=true のまま
	// 残るためである（runners.go の handleOps と同じ規約）。
	c := m.ops.Result(msg)
	return m, tea.Batch(m.chrome(), c)
}

// showLog は Logs タブへ移り、直近ジョブの Worker ログを開くよう親へ求める Cmd を返す。
//
// **page.Do で包まない。** 宛先は親であり、包むと発行元のタブへ戻る
// （page.OpenTabMsg の doc）。
func showLog(r runner.Runner) tea.Cmd {
	return page.OpenTab(page.TabLogs, page.ShowLogMsg{Runner: r})
}
