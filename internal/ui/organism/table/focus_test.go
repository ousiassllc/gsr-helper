package table_test

import (
	"slices"
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/ui/keymap"
	"github.com/ousiassllc/gsr-helper/internal/ui/organism/table"
	"github.com/ousiassllc/gsr-helper/internal/ui/token"
)

// 行の差し替えでカーソルより上の行が消えても、選択は同じ行に留まる。
//
// 3 秒ごとの再検出は runner が 1 台消えるだけで並びを詰めるため、カーソルを生の添字で
// 当て直すと選択が 1 つ下の runner へずれる。フッタの操作可否・enter の詳細・
// サービス制御はいずれも Selected() の結果を対象にするので、ずれは
// 「選んだつもりとは別の runner を操作する」に化ける。
func TestSetItemsKeepsCursorOnSameRow(t *testing.T) {
	tbl := newTable(true, rows("alpha", "bravo", "charlie"))
	tbl, _ = send(tbl, "j")
	if got := selectedName(tbl); got != "bravo" {
		t.Fatalf("前提が崩れている: カーソル = %q, want bravo", got)
	}

	// カーソルより上の alpha が消える。
	tbl.SetItems(0, rows("bravo", "charlie"))
	if got := selectedName(tbl); got != "bravo" {
		t.Errorf("上の行が消えた後のカーソル = %q, want bravo", got)
	}

	// 下の行が消えても動かない。
	tbl.SetItems(0, rows("bravo"))
	if got := selectedName(tbl); got != "bravo" {
		t.Errorf("下の行が消えた後のカーソル = %q, want bravo", got)
	}

	// 上に行が増えても動かない。
	tbl.SetItems(0, rows("alpha", "bravo", "charlie"))
	if got := selectedName(tbl); got != "bravo" {
		t.Errorf("上に行が増えた後のカーソル = %q, want bravo", got)
	}
}

// 選択していた行そのものが消えたら先頭へ戻す。
//
// 添字を据え置くと、たまたまその位置に来た別の行を選んだことになる。先頭は
// 「利用者が改めて選び直す」ことが分かる位置であり、破壊的操作の誤爆を避けられる。
func TestSetItemsResetsCursorWhenRowDisappears(t *testing.T) {
	tbl := newTable(true, rows("alpha", "bravo", "charlie"))
	tbl, _ = send(tbl, "j")

	tbl.SetItems(0, rows("alpha", "charlie"))
	if got := selectedName(tbl); got != "alpha" {
		t.Errorf("選択行が消えた後のカーソル = %q, want alpha", got)
	}
}

// 複数選択も識別子で保たれる。行が入れ替わっても選んだ対象は変わらない。
func TestSetItemsKeepsCheckedRowsByID(t *testing.T) {
	tbl := newTable(true, rows("alpha", "bravo", "charlie"))
	tbl, _ = send(tbl, "j", "space", "j", "space") // bravo と charlie を選択

	if got, want := names(tbl.Checked()), []string{"bravo", "charlie"}; !slices.Equal(got, want) {
		t.Fatalf("前提が崩れている: 選択 = %v, want %v", got, want)
	}

	tbl.SetItems(0, rows("bravo", "charlie"))
	if got, want := names(tbl.Checked()), []string{"bravo", "charlie"}; !slices.Equal(got, want) {
		t.Errorf("上の行が消えた後の選択 = %v, want %v", got, want)
	}
	if got := selectedName(tbl); got != "charlie" {
		t.Errorf("上の行が消えた後のカーソル = %q, want charlie", got)
	}
}

// 絞り込みで行が減っても、残っていればカーソルは同じ行に留まる。
func TestFilterKeepsCursorOnSameRow(t *testing.T) {
	tbl := newTable(true, rows("alpha", "bravo", "bravo-2"))
	tbl, _ = send(tbl, "j", "j")
	if got := selectedName(tbl); got != "bravo-2" {
		t.Fatalf("前提が崩れている: カーソル = %q, want bravo-2", got)
	}

	tbl, _ = send(tbl, "/", "b", "r", "a", "v", "o", "enter")
	if got := selectedName(tbl); got != "bravo-2" {
		t.Errorf("絞り込み後のカーソル = %q, want bravo-2", got)
	}
}

// 識別子を返す関数を持たない区画でも、行の差し替えで落ちない（添字に退避する）。
func TestSetItemsWithoutIDFallsBackToIndex(t *testing.T) {
	sec := runnerSection(false)
	sec.ID = nil
	tbl := table.New(keymap.NewList(), token.NewStyles(true, false), sec)
	tbl.SetSize(80, 12)
	tbl.SetItems(0, rows("alpha", "bravo", "charlie"))
	tbl, _ = send(tbl, "j")

	tbl.SetItems(0, rows("alpha", "bravo"))
	if got := selectedName(tbl); got != "bravo" {
		t.Errorf("識別子の無い区画のカーソル = %q, want bravo", got)
	}
}
