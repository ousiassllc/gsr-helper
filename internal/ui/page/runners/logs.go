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

// handleResult は詳細画面が返した決定を処理する。
//
// この版で実行できるのは `l`（ログを開く）だけである。残る操作は
// action.Def.Supported が偽なので ChoiceList が決定を発行しない。操作を実装する
// Issue はここに分岐を足す。
func (m Model) handleResult(msg page.ResultMsg) (tea.Model, tea.Cmd) {
	chosen, ok := msg.Msg.(runnerdetail.ChosenMsg)
	if !ok || chosen.Action != action.Logs {
		return m, m.chrome()
	}
	// 詳細画面は閉じる。Logs タブへ移ったあとも開いたままだと、戻ってきたときに
	// 別の runner の詳細が残る。
	m.overlay.Close()
	return m, tea.Batch(m.chrome(), showLog(chosen.Runner))
}

// showLog は Logs タブへ移り、直近ジョブの Worker ログを開くよう親へ求める Cmd を返す。
//
// **page.Do で包まない。** 宛先は親であり、包むと発行元のタブへ戻る
// （page.OpenTabMsg の doc）。
func showLog(r runner.Runner) tea.Cmd {
	return page.OpenTab(page.TabLogs, page.ShowLogMsg{Runner: r})
}
