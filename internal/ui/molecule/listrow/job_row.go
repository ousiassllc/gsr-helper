package listrow

import (
	"strconv"
	"time"

	"github.com/ousiassllc/gsr-helper/internal/ui/atom"
	"github.com/ousiassllc/gsr-helper/internal/ui/token"
)

// JobView は実行中ジョブ 1 行の表示用の構造体。
type JobView struct {
	Runner     string
	Repository string
	Elapsed    time.Duration
	WorkerPID  int
	Work       string
}

// JobRow は cols と同じ順・同じ数のセルを返す。
func JobRow(v JobView, cols []token.Column, s token.Styles) []string {
	cells := make([]string, 0, len(cols))
	for _, c := range cols {
		cells = append(cells, jobCell(v, c, s))
	}
	return cells
}

// jobCell は列 1 つ分のセルを返す。
func jobCell(v JobView, c token.Column, s token.Styles) string {
	switch c.ID {
	case token.ColRunner:
		return dashCell(v.Runner, c, s)
	case token.ColRepository:
		return dashCell(v.Repository, c, s)
	case token.ColElapsed:
		return atom.Cell(atom.Duration(v.Elapsed), c.Width, columnAlign(c))
	case token.ColWorkerPID:
		return jobPIDCell(v.WorkerPID, c, s)
	case token.ColWork:
		// パスは中間を中略して末尾（ジョブごとに変わる部分）を残す。
		return dashCell(atom.Path(v.Work, c.Width), c, s)
	default:
		return atom.Cell("", c.Width, columnAlign(c))
	}
}

// jobPIDCell は Worker のプロセス ID を右寄せのセルで返す。
// 取得できていない場合は「値なし」の記号にする。
func jobPIDCell(pid int, c token.Column, s token.Styles) string {
	if pid <= 0 {
		return dashCell("", c, s)
	}
	return atom.Cell(strconv.Itoa(pid), c.Width, columnAlign(c))
}
