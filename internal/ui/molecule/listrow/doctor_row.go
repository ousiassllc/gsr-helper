package listrow

import (
	"github.com/ousiassllc/gsr-helper/internal/ui/atom"
	"github.com/ousiassllc/gsr-helper/internal/ui/token"
)

// DoctorView は Doctor タブの診断結果 1 行の表示用の構造体。
//
// ドメインの型（doctor.CheckResult）ではなく表示用の値を受け取る。molecule 以下は
// プリミティブと表示用の構造体だけを扱う（atomic-design.md の依存の規則）。
type DoctorView struct {
	// Status は判定。
	Status token.StateToken
	// Category は分類（「ジョブ実行の前提」）。
	Category string
	// Check は要約（「docker グループが未反映」）。
	Check string
	// Target は対象 runner。ホスト全体のチェックでは空。
	Target string
}

// DoctorRow は cols と同じ順・同じ数のセルを返す。
func DoctorRow(v DoctorView, cols []token.Column, s token.Styles) []string {
	cells := make([]string, 0, len(cols))
	for _, c := range cols {
		cells = append(cells, doctorCell(v, c, s))
	}
	return cells
}

// doctorCell は列 1 つ分のセルを返す。
func doctorCell(v DoctorView, c token.Column, s token.Styles) string {
	switch c.ID {
	case token.ColStatus:
		text, role := atom.DoctorStatus(v.Status)
		return styledCell(text, role, c, s)
	case token.ColCategory:
		return dashCell(v.Category, c, s)
	case token.ColCheck:
		// 要約は判定と同じ色で描く。**行の意味を決めるのはこの列**であり、
		// STATUS が幅で落ちない列だとしても、要約だけを目で追えるようにする。
		return styledCell(v.Check, v.Status.Role(), c, s)
	case token.ColTarget:
		return dashCell(v.Target, c, s)
	default:
		return atom.Cell("", c.Width, columnAlign(c))
	}
}
