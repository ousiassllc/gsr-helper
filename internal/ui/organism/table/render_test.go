package table_test

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/ousiassllc/gsr-helper/internal/ui/keymap"
	"github.com/ousiassllc/gsr-helper/internal/ui/organism/table"
)

// RenderRow が返すセルの数と列の数が食い違ったときの、詰めと切り捨てを検証する。

// 行のセル数が列数と違っても panic せず、列数ぶんに揃う（理由は fitCells を参照）。
func TestTableToleratesRenderRowCellCountMismatch(t *testing.T) {
	tests := map[string]struct {
		render table.RenderRow[row]
		// want は 1 行に出る名前の数。Render は全セルに名前を入れるので、詰め・
		// 切り捨ての後に残ったセルの数がそのまま数えられる。列は 2 つである。
		want int
	}{
		"列数より多い": {
			render: func(in table.RowInput[row]) []string {
				cells := make([]string, 0, len(in.Cols)+3)
				for range len(in.Cols) + 3 {
					cells = append(cells, in.Item.name)
				}
				return cells
			},
			want: 2, // 余った 3 つは落ちる
		},
		"列数より少ない": {
			render: func(in table.RowInput[row]) []string { return []string{in.Item.name} },
			want:   1, // 足りない 1 つは空セルで詰められる
		},
		"1 つも返さない": {
			render: func(table.RowInput[row]) []string { return nil },
			want:   0, // 2 つとも空セルで詰められる
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			sec := runnerSection(true)
			sec.Render = tt.render
			tbl := table.New(keymap.NewList(), testStyles(), sec)
			tbl.SetSize(80, 12)
			tbl.SetItems(0, rows("build01-1", "build01-2"))

			// **セルが列数に合わせて詰められた／切られたことまで見る。**
			// View() != "" しか見ていなかった頃は、余ったセルがそのまま描かれても
			// 行が 1 つも出なくても緑のままで、panic しないことしか担保していな
			// かった（Issue #31）。
			lines := strings.Split(tbl.View(), "\n")
			if len(lines) != 3 {
				t.Fatalf("描かれた行数 = %d, want 3（見出し + 2 行）:\n%s", len(lines), tbl.View())
			}
			for i, want := range []string{"build01-1", "build01-2"} {
				line := lines[i+1]
				if got := strings.Count(line, want); got != tt.want {
					t.Errorf("%q の行に出たセルの数 = %d, want %d（%q）", want, got, tt.want, line)
				}
				if got := lipgloss.Width(line); got != 80 {
					t.Errorf("%q の行の幅 = %d, want 80（%q）", want, got, line)
				}
			}

			// 打鍵でも panic しない（カーソル移動・選択・末尾へ）。
			tbl, _ = send(tbl, "j", "space", "G")
			if got := tbl.View(); got == "" {
				t.Error("行があるのに何も描かれていない")
			}
		})
	}
}
