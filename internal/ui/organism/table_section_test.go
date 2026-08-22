package organism_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/ui/keymap"
	"github.com/ousiassllc/gsr-helper/internal/ui/organism"
)

// newSectioned は Runners タブと同じ「一覧 + 孤児ユニット」の 2 区画を組み立てる。
func newSectioned(runners, orphans []row) organism.Table[row] {
	t := organism.NewTable(keymap.NewList(), testStyles(), runnerSection(true), orphanSection())
	t.SetSize(80, 16)
	t.SetItems(0, runners)
	t.SetItems(1, orphans)
	return t
}

// 区画の末尾で j を押すと次の区画の先頭へ、先頭で k を押すと前の区画の末尾へ移る。
//
// 区画をまたぐ移動に専用のキーは無い（keymap.List に NextSection を持たない）。
func TestTableMovesAcrossSections(t *testing.T) {
	tests := map[string]struct {
		keys []string
		want string
	}{
		"区画 0 の末尾へ":      {[]string{"j"}, "build01-2"},
		"区画 1 の先頭へ":      {[]string{"j", "j"}, "old01.service"},
		"区画 0 の末尾へ戻る":    {[]string{"j", "j", "k"}, "build01-2"},
		"最後の区画を越えない":     {[]string{"j", "j", "j", "j", "j"}, "old01.service"},
		"最初の区画を越えない":     {[]string{"j", "j", "k", "k", "k", "k"}, "build01-1"},
		"tab では区画を移動しない": {[]string{"tab"}, "build01-1"},
		"G は最後の区画の末尾へ":   {[]string{"G"}, "old01.service"},
		"g は最初の区画の先頭へ":   {[]string{"G", "g"}, "build01-1"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			tbl, _ := send(newSectioned(rows("build01-1", "build01-2"), rows("old01.service")), tt.keys...)
			if got := selectedName(tbl); got != tt.want {
				t.Errorf("カーソル位置 = %q, want %q", got, tt.want)
			}
		})
	}
}

// 選択できない区画では space を無視する。
func TestTableIgnoresToggleInNonSelectableSection(t *testing.T) {
	tbl := newSectioned(rows("build01-1"), rows("old01.service"))

	tbl, _ = send(tbl, "j")
	if got := selectedName(tbl); got != "old01.service" {
		t.Fatalf("孤児ユニットの区画へ移れていない（%q）", got)
	}

	if got := tbl.FocusedSection(); got != 1 {
		t.Errorf("カーソルがある区画 = %d, want 1", got)
	}

	tbl, _ = send(tbl, "space", "ctrl+a")
	for _, name := range names(tbl.Checked()) {
		if name == "old01.service" {
			t.Error("選択できない区画の行が選択されている")
		}
	}
	if got := len(names(tbl.Checked())); got != 1 {
		t.Errorf("選択件数 = %d, want 1（選択可能な区画の 1 件のみ）", got)
	}
}

// 行が無い区画は飛ばす。区切り線も見出しも出さない。
func TestTableSkipsEmptySections(t *testing.T) {
	tbl := newSectioned(rows("build01-1"), nil)

	if got := tbl.View(); strings.Contains(got, "孤児ユニット") {
		t.Error("行が無い区画の見出しが描かれている")
	}

	tbl, _ = send(tbl, "j")
	if got := selectedName(tbl); got != "build01-1" {
		t.Errorf("空の区画へフォーカスが移った（%q）", got)
	}

	// 行が入れば区画が現れ、またげるようになる。
	tbl.SetItems(1, rows("old01.service"))
	if got := tbl.View(); !strings.Contains(got, "孤児ユニット") {
		t.Error("行が入った区画の見出しが描かれていない")
	}
	tbl, _ = send(tbl, "j")
	if got := selectedName(tbl); got != "old01.service" {
		t.Errorf("行が入った区画へ移れない（%q）", got)
	}

	// 行が無くなった区画からはフォーカスが外れる。
	tbl.SetItems(1, nil)
	if got := selectedName(tbl); got != "build01-1" {
		t.Errorf("空になった区画からフォーカスが外れていない（%q）", got)
	}
}

// 行が戻った区画は行を描く。bubbles/table のカーソルは 0 件で -1 になり行が戻っても -1 の
// ままで、その table は 1 行も描かない。フォーカスの無い区画（孤児ユニット）では
// normalizeFocus が触らないので refresh が先頭へ戻す必要がある。
func TestTableDrawsRowsReturnedToBackgroundSection(t *testing.T) {
	tbl := newSectioned(rows("build01-1"), nil)

	tbl.SetItems(1, rows("old01.service"))
	if got := tbl.View(); !strings.Contains(got, "old01.service") {
		t.Errorf("行が戻った区画が描かれていない: %q", got)
	}
}

// View は与えられた高さを超えない。区画の最低行数の合計が高さを超える場合も、
// 割り当てられた領域から出ない（枠の切り詰めに頼らない）。
func TestTableViewNeverExceedsHeight(t *testing.T) {
	for _, h := range []int{1, 2, 3, 4, 5, 8, 16} {
		tbl := newSectioned(rows("build01-1", "build01-2"), rows("old01.service"))
		tbl.SetSize(80, h)
		if got := len(strings.Split(tbl.View(), "\n")); got > h {
			t.Errorf("高さ %d のときの行数 = %d", h, got)
		}
	}
}

// 絞り込みは区画ごとに効く。一致判定を持たない区画は絞り込まない。
func TestTableFiltersEachSection(t *testing.T) {
	tbl := newSectioned(rows("build01-1", "web-1"), rows("old01.service"))

	tbl, _ = send(tbl, "/", "w", "e", "b", "enter")

	if got := names(tbl.Shown(0)); !slices.Equal(got, []string{"web-1"}) {
		t.Errorf("絞り込み後の表示 = %v, want [web-1]", got)
	}
	if got := names(tbl.Shown(1)); !slices.Equal(got, []string{"old01.service"}) {
		t.Errorf("一致判定を持たない区画の表示 = %v, want [old01.service]", got)
	}
}
