package listrow

import (
	"github.com/ousiassllc/gsr-helper/internal/ui/atom"
	"github.com/ousiassllc/gsr-helper/internal/ui/token"
)

// OrphanView は孤児ユニット 1 行の表示用の構造体。
//
// 孤児ユニットは対応する runner ディレクトリが無いため、名前とサービス状態、
// そして「なぜ孤児なのか」の注記だけを持つ。
type OrphanView struct {
	Unit   string // ユニット名
	Active string // systemctl の ActiveState
	Sub    string // systemctl の SubState
	Note   string // 注記（「（対応ディレクトリなし）」など）
}

// OrphanRow は cols と同じ順・同じ数のセルを返す。
func OrphanRow(v OrphanView, cols []token.Column, s token.Styles) []string {
	cells := make([]string, 0, len(cols))
	for _, c := range cols {
		cells = append(cells, orphanCell(v, c, s))
	}
	return cells
}

// orphanCell は列 1 つ分のセルを返す。
func orphanCell(v OrphanView, c token.Column, s token.Styles) string {
	switch c.ID {
	case token.ColUnit:
		return dashCell(v.Unit, c, s)
	case token.ColSvc:
		text, role := atom.StatusText(v.Active, v.Sub)
		return styledCell(text, role, c, s)
	case token.ColNote:
		if v.Note == "" {
			return dashCell("", c, s)
		}
		// 注記は補足情報なので薄く描く。
		return styledCell(v.Note, token.RoleMuted, c, s)
	default:
		return atom.Cell("", c.Width, columnAlign(c))
	}
}
