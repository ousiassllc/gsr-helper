package listrow

import (
	"strconv"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/ousiassllc/gsr-helper/internal/ui/molecule"
	"github.com/ousiassllc/gsr-helper/internal/ui/token"
)

// plainStyles は色を使わないスタイル。期待値を素の文字列で書けるようにする。
// 共有するテストヘルパーは internal/exec/helper_test.go と同じくここに集める。
func plainStyles() token.Styles {
	return token.NewStyles(true, false)
}

// assertRowCells は行系 molecule が守る約束を検証する。
//
// 約束は 2 つある。セル数が列数と一致すること（超えると bubbles/table が添字
// 範囲外で panic する）と、各セルの幅が列幅と一致すること（ヘッダと桁が揃う）。
// RunnerRow / OrphanRow / JobRow で同じなので共通化する。
func assertRowCells(t *testing.T, name string, cols []token.Column, cells []string) {
	t.Helper()

	if len(cells) != len(cols) {
		t.Errorf("%s: セル数 = %d, want %d", name, len(cells), len(cols))
		return
	}
	for i, c := range cols {
		if got := lipgloss.Width(cells[i]); got != c.Width {
			t.Errorf("%s: 列 %s のセル幅 = %d, want %d（%q）", name, c.ID, got, c.Width, cells[i])
		}
	}
}

// assertRows は幅ごとに列を選び、各 view の行が assertRowCells の約束を守ることを
// 検証する。列が無い場合に 0 件を返すことも併せて確かめる。
func assertRows[T any](
	t *testing.T,
	all []token.Column,
	widths []int,
	views map[string]T,
	render func(v T, cols []token.Column, s token.Styles) []string,
) {
	t.Helper()

	for _, width := range widths {
		cols := molecule.Columns(all, width, token.RunnerColumnRules())
		for name, v := range views {
			assertRowCells(t, "幅 "+strconv.Itoa(width)+" "+name, cols, render(v, cols, plainStyles()))
		}
	}

	// 列が無い場合も panic せずセルを返さない。
	var zero T
	if got := render(zero, nil, plainStyles()); len(got) != 0 {
		t.Errorf("列が無いときのセル数 = %d, want 0", len(got))
	}
}
