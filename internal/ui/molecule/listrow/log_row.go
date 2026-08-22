package listrow

import (
	"time"

	"github.com/ousiassllc/gsr-helper/internal/ui/atom"
	"github.com/ousiassllc/gsr-helper/internal/ui/token"
)

// LogView は Logs タブのファイル一覧 1 行の表示用の構造体。
//
// ドメインの型（logs.File）ではなく表示用の値を受け取る。molecule 以下は
// プリミティブと表示用の構造体だけを扱う（atomic-design.md の依存の規則）。
type LogView struct {
	// Runner は所有する runner の名前。
	Runner string
	// Name はログのファイル名。
	Name string
	// Size はバイト数。
	Size int64
	// Updated は更新時刻。ゼロ値は「取得できていない」を表す。
	Updated time.Time
}

// updatedLayout は UPDATED 列の表記。列幅（16）ちょうどに収まる。
//
// 年を落として時刻の精度を上げないのは、`_diag` のログが数日〜数週間ぶん残るため
// である。日付が無いと、同じ時刻の別の日のログを取り違える。
const updatedLayout = "2006-01-02 15:04"

// LogRow は cols と同じ順・同じ数のセルを返す。
func LogRow(v LogView, cols []token.Column, s token.Styles) []string {
	cells := make([]string, 0, len(cols))
	for _, c := range cols {
		cells = append(cells, logCell(v, c, s))
	}
	return cells
}

// logCell は列 1 つ分のセルを返す。
func logCell(v LogView, c token.Column, s token.Styles) string {
	switch c.ID {
	case token.ColRunner:
		return dashCell(v.Runner, c, s)
	case token.ColLog:
		return dashCell(v.Name, c, s)
	case token.ColSize:
		return atom.Cell(atom.Bytes(v.Size), c.Width, columnAlign(c))
	case token.ColUpdated:
		return logUpdatedCell(v.Updated, c, s)
	default:
		return atom.Cell("", c.Width, columnAlign(c))
	}
}

// logUpdatedCell は更新時刻のセルを返す。取得できていない場合は「値なし」の記号にする。
func logUpdatedCell(t time.Time, c token.Column, s token.Styles) string {
	if t.IsZero() {
		return dashCell("", c, s)
	}
	return styledCell(t.Format(updatedLayout), token.RolePlain, c, s)
}
