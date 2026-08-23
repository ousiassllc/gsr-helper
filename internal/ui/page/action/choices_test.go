package action

// 詳細画面の操作リスト（organism.Choice）の組み立てに関する検査を集める。
//
// allow_test.go から分けているのは、1 ファイル 300 行の上限（.linterly.yml）に
// 収めるためである。可否の判定そのものは allow_test.go が持つ。

import (
	"slices"
	"testing"
)

// 操作リストは区切り線を 1 本だけ持ち、最初の破壊的な操作の前に置く。
func TestChoicesDivider(t *testing.T) {
	items := testActions().Choices(sampleRunner(), fullCaps())
	acts := testActions().List()
	if len(items) != len(acts) {
		t.Fatalf("項目の件数 = %d, want %d", len(items), len(acts))
	}

	dividers := 0
	for i, c := range items {
		if !c.DividerBefore {
			continue
		}
		dividers++
		if !acts[i].Destructive {
			t.Errorf("区切り線が破壊的でない操作 %q の前にある", c.Key)
		}
		if i > 0 && acts[i-1].Destructive {
			t.Errorf("区切り線が最初の破壊的な操作より後（%q の前）にある", c.Key)
		}
	}
	if dividers != 1 {
		t.Errorf("区切り線の本数 = %d, want 1", dividers)
	}
}

// 影響の併記は確認ダイアログを経る 3 操作（停止・強制停止・再起動）と削除が持つ
// （screens.md の詳細画面のモックの括弧内）。
func TestChoicesImpact(t *testing.T) {
	withImpact := []string{}
	for _, c := range testActions().Choices(sampleRunner(), fullCaps()) {
		if c.Impact != "" {
			withImpact = append(withImpact, c.Key)
		}
	}
	slices.Sort(withImpact)
	if want := []string{"D", "R", "X", "x"}; !slices.Equal(withImpact, want) {
		t.Errorf("影響を併記する操作 = %v, want %v", withImpact, want)
	}
}
