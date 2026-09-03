package table_test

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/ousiassllc/gsr-helper/internal/ui/keymap"
	"github.com/ousiassllc/gsr-helper/internal/ui/organism/table"
)

// RenderRow が返すセルの数が列数を超えたときの切り捨てを検証する。
//
// 詰め（足りない分を空セルで埋める）は View() から観測できないので、ここでは見ない
// （fitcells_test.go が白箱で固定している）。

// 行のセル数が列数と違っても panic せず、列数を超えない（理由は fitCells を参照）。
func TestTableToleratesRenderRowCellCountMismatch(t *testing.T) {
	tests := map[string]struct {
		render table.RenderRow[row]
		// want は 1 行に出る名前の数。Render は全セルに名前を入れるので、切り捨ての
		// 後に描画へ出たセルの数がそのまま数えられる。列は 2 つである。
		want int
	}{
		"列数より多い": {
			render: func(in table.RowInput[row]) []string {
				cells := make([]string, 0, len(in.Cols)+3)
				for range len(in.Cols) + 3 {
					cells = append(cells, in.Item.Name)
				}
				return cells
			},
			want: 2, // 余った 3 つは落ちる（切り捨て）
		},
		"列数より少ない": {
			render: func(in table.RowInput[row]) []string { return []string{in.Item.Name} },
			want:   1, // 足りない 1 つは描画に出ない
		},
		"1 つも返さない": {
			render: func(table.RowInput[row]) []string { return nil },
			want:   0, // 名前はどの列にも出ない
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			sec := runnerSection(true)
			sec.Render = tt.render
			tbl := table.New(keymap.NewList(), testStyles(), sec)
			tbl.SetSize(80, 12)
			tbl.SetItems(0, rows("build01-1", "build01-2"))

			// **セルが列数に合わせて切られたことまで見る。**
			//
			// 詰め（足りないセルを空文字で埋める）はここからは観測できない。埋めても
			// 埋めなくても bubbles/table が行を幅いっぱいに伸ばすので出力が同じに
			// なるためで、詰めは fitcells_test.go が白箱で固定している。
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
