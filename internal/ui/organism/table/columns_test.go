package table_test

import (
	"slices"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/ousiassllc/gsr-helper/internal/ui/organism/table"
	"github.com/ousiassllc/gsr-helper/internal/ui/token"
)

// View の各行は SetSize に渡した幅を超えない。
//
// 行頭の幅（organism のガター列）と molecule の幅判定（columnPrefix / columnGutter）が
// 食い違うと、保証する幅でも行が折り返して枠の行数を固定した意味が失われる。両者の
// 突き合わせを検証できるのはこのテストだけである。
//
// 絞り込みの行も同じ不変条件を満たす。絞り込み文字列は利用者が好きな長さを打てるため、
// 入力中と確定後の両方を通す（切り詰めの理由は filterView を参照）。
func TestTableViewNeverExceedsWidth(t *testing.T) {
	// 選択中はチェックボックスが出るので、いずれの状態でも選択してから確かめる。
	modes := map[string][]string{
		"絞り込みなし": {"ctrl+a"},
		"入力中":    append([]string{"ctrl+a"}, longFilterKeys(false)...),
		"確定後":    append([]string{"ctrl+a"}, longFilterKeys(true)...),
	}
	for name, cols := range columnSets() {
		for _, selectable := range []bool{true, false} {
			for mode, keys := range modes {
				// 1 つの一覧をリサイズして回す。幅ごとに組み直すとキー列の長さだけ
				// 実行時間が伸びるうえ、リサイズを挟んでも幅が保たれることまで確かめられる。
				tbl, _ := send(newColumned(cols, selectable, nil), keys...)
				for w := token.WidthMin; w <= 120; w++ {
					tbl.SetSize(w, 12)
					for i, line := range strings.Split(tbl.View(), "\n") {
						if got := lipgloss.Width(line); got > w {
							t.Fatalf("%s selectable=%v %s 幅 %d: %d 行目の表示幅 = %d",
								name, selectable, mode, w, i, got)
						}
					}
				}
			}
		}
	}
}

// SetSize で幅を狭めると採用列が減り、広げると戻る。そのときカーソル位置・
// 選択集合・絞り込み文字列は保たれる（作り直すと失われるため）。
func TestTableResolvesColumnsOnSetSize(t *testing.T) {
	var seen []string
	tbl := newColumned(token.RunnerColumns(), true,
		func(in table.RowInput[row]) []string {
			seen = columnIDs(in.Cols)
			return renderRow(in)
		})

	tbl.SetSize(120, 12)
	tbl, _ = send(tbl, "j", "space", "/", "0", "1", "enter")
	wide := slices.Clone(seen)
	if !slices.Contains(wide, token.ColWork) {
		t.Fatalf("幅 120 で採用された列 = %v, want _WORK を含む", wide)
	}

	tbl.SetSize(70, 12)
	narrow := slices.Clone(seen)
	if slices.Contains(narrow, token.ColWork) || len(narrow) >= len(wide) {
		t.Errorf("幅 70 で採用された列 = %v, want %v より少なく _WORK を含まない", narrow, wide)
	}
	if got := selectedName(tbl); got != "build01-2" {
		t.Errorf("リサイズでカーソルが動いた（%q）", got)
	}
	if got := names(tbl.Checked()); !slices.Equal(got, []string{"build01-2"}) {
		t.Errorf("リサイズで選択が失われた（%v）", got)
	}
	if got := tbl.FilterValue(); got != "01" {
		t.Errorf("リサイズで絞り込みが失われた（%q）", got)
	}

	tbl.SetSize(120, 12)
	if !slices.Equal(seen, wide) {
		t.Errorf("幅を戻したときの列 = %v, want %v", seen, wide)
	}
}

// 大きさが変わらない SetSize では列を解き直さない（行を作り直さない。理由は SetSize の
// doc。1 周期で全タブ・全区画ぶんの全行再構築が積み上がる）。
func TestTableSetSizeSkipsUnchangedSize(t *testing.T) {
	renders := 0
	tbl := newColumned(token.RunnerColumns(), true,
		func(in table.RowInput[row]) []string {
			renders++
			return renderRow(in)
		})

	base := renders
	for range 4 {
		tbl.SetSize(80, 12)
	}
	if renders != base {
		t.Errorf("同じ大きさの SetSize で行を %d 回作り直している", renders-base)
	}

	// 大きさが変わったときは解き直し、カーソル位置は保つ。
	if tbl.SetSize(70, 12); renders == base || selectedName(tbl) != "build01-1" {
		t.Errorf("再構築 %d 回 / カーソル %q（大きさの変更が反映されていない）",
			renders-base, selectedName(tbl))
	}
}
