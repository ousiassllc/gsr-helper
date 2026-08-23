package jobs

import (
	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/ui/page"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/action"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/runnerdetail"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/runnerop"
)

// Jobs タブの組み立て。jobs.go から分けているのは 1 ファイル 300 行の上限に収める
// ためであり、責務の境界ではない。

// New は Jobs タブを組み立てる。tab は親が持つタブ番号で、ChromeMsg に載せる。
//
// モーダルは画面が登録する（page.Overlay の doc）。Jobs タブが開くのは runner の
// 詳細画面だけで、操作対象がジョブではなく runner であることと対応する（FR-47）。
func New(tab int, st page.StateMsg) Model {
	// ヘルプと詳細画面、どちらの登録が返した Cmd も畳み込む（runners.go と同じ理由）。
	overlay, help := page.NewOverlay(tab, st)
	detail := overlay.Register(runnerdetail.Kind, runnerdetail.New(st))
	// 確認ダイアログと待機画面の登録は runnerop が行う（runners.go と同じ理由）。
	ops, opsCmd := runnerop.New(tab, overlay, st)
	cmd := tea.Batch(help, detail, opsCmd)
	return Model{
		tab:     tab,
		st:      st,
		tbl:     newTable(st.Keys, st.Styles),
		overlay: overlay,
		actions: action.NewSet(st.Keys.Runner, st.Scopes),
		ops:     ops,
		initCmd: cmd,
	}
}
