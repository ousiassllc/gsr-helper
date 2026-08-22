package organism_test

import (
	"slices"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/ousiassllc/gsr-helper/internal/ui/keymap"
	"github.com/ousiassllc/gsr-helper/internal/ui/organism"
	"github.com/ousiassllc/gsr-helper/internal/ui/token"
)

// columnSets は実際に使う列の組み合わせを返す。
func columnSets() map[string][]token.Column {
	return map[string][]token.Column{
		"Runner": token.RunnerColumns(),
		"Job":    token.JobColumns(),
		"Orphan": token.OrphanColumns(),
	}
}

// newColumned は列の全集合を持つ 1 区画の一覧を組み立てる。
func newColumned(cols []token.Column, selectable bool, r organism.RenderRow[row]) organism.Table[row] {
	sec := runnerSection(selectable)
	sec.Columns = cols
	if r != nil {
		sec.Render = r
	}

	t := organism.NewTable(keymap.NewList(), testStyles(), sec)
	t.SetSize(80, 12)
	t.SetItems(0, rows("build01-1", "build01-2", "build02-1"))
	return t
}

// longFilterKeys は絞り込みを始めて領域より長い文字列を打つキー列を返す。accept が真なら
// enter で確定する。1 文字で 2 桁使う全角で埋めるのは、検証する最大の幅（120）を短い
// キー列で超えるためである。
func longFilterKeys(accept bool) []string {
	keys := []string{"/"}
	for range 70 {
		keys = append(keys, "長")
	}
	if accept {
		keys = append(keys, "enter")
	}
	return keys
}

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
		func(r row, cols []token.Column, s token.Styles) []string {
			seen = columnIDs(cols)
			return renderRow(r, cols, s)
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
		func(r row, cols []token.Column, s token.Styles) []string {
			renders++
			return renderRow(r, cols, s)
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
