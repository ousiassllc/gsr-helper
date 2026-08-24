package docscheck

import (
	"fmt"
	"regexp"
	"strings"
)

// proseBacktick は散文の backtick 引用を拾う。ディレクトリ名の候補になる。
var proseBacktick = regexp.MustCompile("`([^`]+)`")

// budgetRowIndex は行数表の行をディレクトリ名で引ける索引である。
//
// 表と散文でパスの書き方が揃っていない。UI 層の表は `internal/` 抜きで書き
// （`ui` / `ui/page/disk`）、UI 層の外の表は `internal/...` / `cmd/...` と書く。
// 一方で散文は `internal/ui/page/` のように `internal/` 付き・末尾スラッシュ付きでも
// 書く。索引はその表記ゆれを同じ行へ向ける。
type budgetRowIndex map[string]budgetRow

// newBudgetRowIndex は 2 つの行数表から索引を作る。
func newBudgetRowIndex(uiRows, nonUIRows []budgetRow) budgetRowIndex {
	idx := make(budgetRowIndex, 2*len(uiRows)+len(nonUIRows))
	for _, row := range uiRows {
		idx[row.dir] = row
		idx["internal/"+row.dir] = row
	}
	for _, row := range nonUIRows {
		idx[row.dir] = row
	}
	return idx
}

// namedBudgetRows は文が backtick で名指ししたディレクトリの行を、文に現れた順で返す。
//
// backtick の中身はディレクトリ名とは限らない（`internal/ui/app_test.go` のような
// ファイル名、`page.StateMsg` のような識別子、コマンド）。表の行に当たらない引用は
// 落とすので、1 つも当たらなければ nil を返す——呼び手はそのとき表全体へ落ちる。
func (idx budgetRowIndex) namedBudgetRows(sentence string) []budgetRow {
	var rows []budgetRow
	seen := map[string]bool{}
	for _, m := range proseBacktick.FindAllStringSubmatch(sentence, -1) {
		row, ok := idx[strings.TrimSuffix(strings.TrimSpace(m[1]), "/")]
		if !ok || seen[row.dir] {
			continue
		}
		seen[row.dir] = true
		rows = append(rows, row)
	}
	return rows
}

// budgetRowsLabel は突き合わせた相手をエラーメッセージに書ける形にする。
func budgetRowsLabel(rows []budgetRow) string {
	parts := make([]string, 0, len(rows))
	for _, row := range rows {
		parts = append(parts, fmt.Sprintf("`%s`（%d 行・残り %d 行）", row.dir, row.lines, row.remaining))
	}
	return strings.Join(parts, " / ")
}
