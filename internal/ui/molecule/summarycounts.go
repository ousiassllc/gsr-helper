package molecule

import (
	"strconv"
	"strings"

	"github.com/ousiassllc/gsr-helper/internal/ui/atom"
	"github.com/ousiassllc/gsr-helper/internal/ui/token"
)

// SummaryView は判定ごとの件数（screens.md の `OK 12  WARN 3  FAIL 3  SKIP 1`）。
type SummaryView struct {
	OK   int
	Warn int
	Fail int
	Skip int
}

// summarySep は件数の区切り。見出し行の列間と揃えて空白 2 つにする。
const summarySep = "  "

// SummaryCounts は判定ごとの件数を 1 行にして返す。
//
// **0 件の判定も出す。** 「FAIL 0」が消えると、FAIL が無いのか数え忘れているのかを
// 画面から区別できない。診断は「見たうえで問題が無かった」ことを示すのが目的なので、
// 0 を省いて短くする方は採らない。
//
// 記号を付けないのは、ここが状態の表示ではなく件数の要約だからである。行そのものの
// 判定は一覧の STATUS 列が記号付きで出す（設計原則 4 は状態の判別について定めており、
// 集計の見出しには掛からない）。
func SummaryCounts(v SummaryView, width int, s token.Styles) string {
	parts := []string{
		s.OK.Render(count("OK", v.OK)),
		s.Warn.Render(count("WARN", v.Warn)),
		s.Fail.Render(count("FAIL", v.Fail)),
		s.Skip.Render(count("SKIP", v.Skip)),
	}
	return atom.Join(parts, summarySep, width, token.IconEllipsis)
}

// count は 1 つの判定の表記を返す。
func count(label string, n int) string {
	return label + " " + strconv.Itoa(n)
}

// SummaryLine は件数と最終実行時刻を左右に振り分けた見出し行を返す。
//
// 左右に分けるのは screens.md の Doctor タブの見出しに従ったものである。
// updated が空なら右側を出さない（まだ 1 度も実行していない状態）。
func SummaryLine(v SummaryView, updated string, width int, s token.Styles) string {
	left := SummaryCounts(v, width, s)
	if updated == "" {
		return left
	}
	right := s.Muted.Render("最終実行: " + updated)
	return atom.Justify(left, right, width)
}

// TotalNonZero は 0 件でない判定があるかを返す。診断をまだ実行していない状態
// （すべて 0）と、実行して問題が無かった状態を呼び出し側が区別するために使う。
func (v SummaryView) TotalNonZero() bool {
	return v.OK+v.Warn+v.Fail+v.Skip > 0
}

// String は件数を装飾なしで返す。テストと、色を持たない文脈で使う。
func (v SummaryView) String() string {
	return strings.Join([]string{
		count("OK", v.OK), count("WARN", v.Warn),
		count("FAIL", v.Fail), count("SKIP", v.Skip),
	}, summarySep)
}
