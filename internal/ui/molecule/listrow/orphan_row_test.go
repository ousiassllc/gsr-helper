package listrow

import (
	"strings"
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/ui/molecule"
	"github.com/ousiassllc/gsr-helper/internal/ui/token"
)

func sampleOrphan() OrphanView {
	return OrphanView{
		Unit:   "actions.runner.foo-bar.old01.service",
		Active: "failed",
		Sub:    "failed",
		Note:   "（対応ディレクトリなし）",
	}
}

func TestOrphanRowCells(t *testing.T) {
	views := map[string]OrphanView{
		"標準":     sampleOrphan(),
		"空の値":    {},
		"長いユニット": {Unit: strings.Repeat("actions.runner.foo-bar.old01.service", 2)},
		"遷移中":    {Unit: "old01.service", Active: "deactivating", Sub: "stop-sigterm"},
	}
	assertRows(t, token.OrphanColumns(), []int{120, 80, 78, 70, 60}, views, OrphanRow)

	// 知らない列でもセルを欠かさない。
	unknown := []token.Column{{ID: "UNKNOWN", Title: "UNKNOWN", Width: 8, Right: false}}
	assertRowCells(t, "知らない列", unknown, OrphanRow(sampleOrphan(), unknown, plainStyles()))
}

func TestOrphanRowContents(t *testing.T) {
	cols := sectionColumns(token.OrphanColumns(), token.WidthTarget)
	cells := OrphanRow(sampleOrphan(), cols, plainStyles())

	if !strings.HasPrefix(cells[0], "actions.runner.foo-bar.old01.service") {
		t.Errorf("UNIT セルにユニット名が無い: %q", cells[0])
	}
	if want := token.IconFailed + " failed"; !strings.Contains(cells[1], want) {
		t.Errorf("SVC セル = %q, want %q を含む", cells[1], want)
	}
	if want := "（対応ディレクトリなし）"; !strings.Contains(cells[2], want) {
		t.Errorf("NOTE セル = %q, want %q を含む", cells[2], want)
	}

	// 注記が無い場合は「値なし」の記号にする。
	noNote := OrphanRow(OrphanView{Unit: "x.service", Active: "failed"}, cols, plainStyles())
	if got := strings.TrimSpace(noNote[2]); got != token.IconNoUnit {
		t.Errorf("注記が無いときの NOTE セル = %q, want %q", got, token.IconNoUnit)
	}
}

// columnGutter は列と列の間隔。molecule.Columns の幅判定と organism.Table のセル余白が
// 同じ値を持つ（molecule/columns.go と organism/table/section.go の同名の定数）。
// どちらも非公開なので、区画の見え方を再現するここでも同じ値を置く。
const columnGutter = 1

// sectionColumns は organism の区画が bubbles/table へ渡すのと同じ列を返す。
//
// organism の setWidth は、bubbles/table のセルが持つ右余白の分だけ最終列を 1 セル
// 狭めてから表を組む。Columns の結果をそのまま使って中身を検証すると、実際の描画
// では 1 セル足りずに中略されている値が「収まっている」ように見えてしまう。
func sectionColumns(all []token.Column, width int) []token.Column {
	cols := molecule.Columns(all, width, token.RunnerColumnRules())
	if n := len(cols); n > 0 {
		cols[n-1].Width = max(cols[n-1].Width-columnGutter, 0)
	}
	return cols
}
