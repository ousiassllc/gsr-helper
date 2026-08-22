package token

import (
	"testing"

	"charm.land/lipgloss/v2"
)

func TestWidthMinIsBelowTarget(t *testing.T) {
	if WidthMin >= WidthTarget {
		t.Fatalf("WidthMin(%d) は WidthTarget(%d) より小さくなければならない", WidthMin, WidthTarget)
	}
}

// 常に表示する列が落とす順に含まれていると、幅不足で消えてしまう。
func TestColumnsAlwaysAreNotDroppable(t *testing.T) {
	for name, rules := range map[string]ColumnRules{
		"RunnerColumnRules": RunnerColumnRules(),
		"DiskColumnRules":   DiskColumnRules(),
	} {
		drop := make(map[string]bool, len(rules.Drop))
		for _, id := range rules.Drop {
			drop[id] = true
		}
		for _, id := range rules.Keep {
			if drop[id] {
				t.Errorf("%s: 列 %s は常に表示する列だが落とす順に含まれている", name, id)
			}
		}
	}
}

// 列の定義は、表示を保証する幅（WidthTarget）に全列が収まっていなければならない。
//
// 収まらない定義を置くと、幅 80 の端末でも molecule.Columns が列を落とす。各列の
// doc コメントが宣言している必要幅の計算を、定義そのものと突き合わせる。
//
// 行頭とセル間の見積もりは molecule の columnPrefix / columnGutter と同じ値である。
// token から molecule を参照すると import が循環するため、ここでは同じ値を置き、
// 食い違いは列の doc コメントで揃える（RunnerColumns の doc）。
func TestColumnSetsFitTargetWidth(t *testing.T) {
	const (
		prefix = 6 // カーソル 1 + 間隔 1 + チェックボックス 3 + 間隔 1
		gutter = 1 // 列と列の間隔。最終列の後ろには入らない
	)

	for name, cols := range map[string][]Column{
		"RunnerColumns": RunnerColumns(),
		"OrphanColumns": OrphanColumns(),
		"JobColumns":    JobColumns(),
		"DiskColumns":   DiskColumns(),
	} {
		total := prefix + gutter*(len(cols)-1)
		for _, c := range cols {
			total += c.Width
		}
		if total > WidthTarget {
			t.Errorf("%s の必要幅 = %d, want %d 以下", name, total, WidthTarget)
		}
	}
}

// Disk タブの列は、常に表示する列と落とす順の両方を網羅している。
//
// 網羅を確かめるのは、落とす順に載っていない列が末尾から落ちるためである。
// 意図せず「順の宣言から漏れた列」があると、宣言した順とは違う順で消える。
func TestDiskColumnsCoverAlwaysAndDropOrder(t *testing.T) {
	have := make(map[string]bool)
	for _, c := range DiskColumns() {
		have[c.ID] = true
	}
	rules := DiskColumnRules()
	for _, id := range append(rules.Keep, rules.Drop...) {
		if !have[id] {
			t.Errorf("DiskColumns に列 %s がない", id)
		}
	}
	if len(DiskColumns()) != len(rules.Keep)+len(rules.Drop) {
		t.Errorf("DiskColumns の列数 %d が常時表示 %d + 省略対象 %d と一致しない",
			len(DiskColumns()), len(rules.Keep), len(rules.Drop))
	}
}

// Runners タブの列は、常に表示する列と落とす順の両方を網羅している。
func TestRunnerColumnsCoverAlwaysAndDropOrder(t *testing.T) {
	have := make(map[string]bool)
	for _, c := range RunnerColumns() {
		have[c.ID] = true
	}
	for _, id := range append(RunnerColumnRules().Keep, RunnerColumnRules().Drop...) {
		if !have[id] {
			t.Errorf("RunnerColumns に列 %s がない", id)
		}
	}
	if len(RunnerColumns()) != len(RunnerColumnRules().Keep)+len(RunnerColumnRules().Drop) {
		t.Errorf("RunnerColumns の列数 %d が常時表示 %d + 省略対象 %d と一致しない",
			len(RunnerColumns()), len(RunnerColumnRules().Keep), len(RunnerColumnRules().Drop))
	}
}

func TestColumnsAreWellFormed(t *testing.T) {
	sets := map[string][]Column{
		"RunnerColumns": RunnerColumns(),
		"OrphanColumns": OrphanColumns(),
		"JobColumns":    JobColumns(),
		"DiskColumns":   DiskColumns(),
	}
	for name, cols := range sets {
		if len(cols) == 0 {
			t.Errorf("%s が空である", name)
		}
		seen := make(map[string]bool, len(cols))
		for _, c := range cols {
			if c.ID == "" || c.Title == "" {
				t.Errorf("%s に識別子または見出しが空の列がある: %+v", name, c)
			}
			// 幅は表示セル数で見る。rune 数で数えると全角の見出しを過小に数え、
			// 見出しが列幅に収まらない定義を見逃す。
			if c.Width < lipgloss.Width(c.Title) {
				t.Errorf("%s の列 %s は見出し %q より幅が狭い（幅 %d）", name, c.ID, c.Title, c.Width)
			}
			if seen[c.ID] {
				t.Errorf("%s の列 %s が重複している", name, c.ID)
			}
			seen[c.ID] = true
		}
	}
}

// 返り値を書き換えても次の呼び出しに影響しない（var のグローバルにしない理由）。
func TestColumnGettersReturnFreshValues(t *testing.T) {
	cols := RunnerColumns()
	cols[0].Width = 999
	cols[0].Title = "壊れた見出し"
	if again := RunnerColumns(); again[0].Width == 999 || again[0].Title == "壊れた見出し" {
		t.Error("RunnerColumns の返り値への書き換えが次の呼び出しに影響している")
	}

	always := RunnerColumnRules().Keep
	always[0] = "壊れた列"
	if again := RunnerColumnRules().Keep; again[0] == "壊れた列" {
		t.Error("ColumnsAlways の返り値への書き換えが次の呼び出しに影響している")
	}

	order := RunnerColumnRules().Drop
	order[0] = "壊れた列"
	if again := RunnerColumnRules().Drop; again[0] == "壊れた列" {
		t.Error("ColumnDropOrder の返り値への書き換えが次の呼び出しに影響している")
	}
}
