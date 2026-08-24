package disk

import (
	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/disk"
	"github.com/ousiassllc/gsr-helper/internal/ui/organism/pane"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/disk/cleanview"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/progressmodal"
)

// クリーンアップの進捗表示（Issue #75）と、ドメインへ渡す値の取り出しを集める。
// 文面と進捗行の組み立てそのものは cleanview（純粋関数）が持ち、ここにあるのは
// Model の状態を読む薄い写しだけである。

// checkedUsage は選択された行の集計結果だけを取り出す。
//
// 行（row）は表の都合を持つ page の型なので、削除計画の組み立て（cleanview.Targets）
// へはドメインの値だけを渡す。
func checkedUsage(rows []row) []disk.Usage {
	out := make([]disk.Usage, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.usage)
	}
	return out
}

// progressInput は進捗表示へ送る中身を組み立てる。実行中の中身は Job が持つ。
func (m Model) progressInput() pane.ProgressInput {
	if m.clean == nil {
		return pane.ProgressInput{Title: cleanview.ProgressTitle}
	}
	return m.clean.Input()
}

// openProgress は進捗表示を開く。
func (m Model) openProgress() tea.Cmd {
	return progressmodal.Open(&m.overlay, m.progressInput())
}

// updateProgress は開いている進捗表示へ現在の状態を送る。
//
// 開き直すのではなく同じ種類へ送るのは、Overlay の push が同じ種類を二重に積まない
// （既にあれば最前面へ移すだけ）ためである（page/setup の updateProgress と同じ理由）。
func (m Model) updateProgress() tea.Cmd {
	return progressmodal.Set(&m.overlay, m.progressInput())
}
