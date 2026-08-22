package token

import "testing"

func TestWidthMinIsBelowTarget(t *testing.T) {
	if WidthMin >= WidthTarget {
		t.Fatalf("WidthMin(%d) は WidthTarget(%d) より小さくなければならない", WidthMin, WidthTarget)
	}
}

// 常に表示する列が落とす順に含まれていると、幅不足で消えてしまう。
func TestColumnsAlwaysAreNotDroppable(t *testing.T) {
	drop := make(map[string]bool, len(ColumnDropOrder()))
	for _, id := range ColumnDropOrder() {
		drop[id] = true
	}
	for _, id := range ColumnsAlways() {
		if drop[id] {
			t.Errorf("列 %s は常に表示する列だが落とす順に含まれている", id)
		}
	}
}

// Runners タブの列は、常に表示する列と落とす順の両方を網羅している。
func TestRunnerColumnsCoverAlwaysAndDropOrder(t *testing.T) {
	have := make(map[string]bool)
	for _, c := range RunnerColumns() {
		have[c.ID] = true
	}
	for _, id := range append(ColumnsAlways(), ColumnDropOrder()...) {
		if !have[id] {
			t.Errorf("RunnerColumns に列 %s がない", id)
		}
	}
	if len(RunnerColumns()) != len(ColumnsAlways())+len(ColumnDropOrder()) {
		t.Errorf("RunnerColumns の列数 %d が常時表示 %d + 省略対象 %d と一致しない",
			len(RunnerColumns()), len(ColumnsAlways()), len(ColumnDropOrder()))
	}
}

func TestColumnsAreWellFormed(t *testing.T) {
	sets := map[string][]Column{
		"RunnerColumns": RunnerColumns(),
		"OrphanColumns": OrphanColumns(),
		"JobColumns":    JobColumns(),
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
			if c.Width < len([]rune(c.Title)) {
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

	always := ColumnsAlways()
	always[0] = "壊れた列"
	if again := ColumnsAlways(); again[0] == "壊れた列" {
		t.Error("ColumnsAlways の返り値への書き換えが次の呼び出しに影響している")
	}

	order := ColumnDropOrder()
	order[0] = "壊れた列"
	if again := ColumnDropOrder(); again[0] == "壊れた列" {
		t.Error("ColumnDropOrder の返り値への書き換えが次の呼び出しに影響している")
	}
}
