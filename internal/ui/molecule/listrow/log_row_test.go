package listrow

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ousiassllc/gsr-helper/internal/ui/molecule"
	"github.com/ousiassllc/gsr-helper/internal/ui/token"
)

func sampleLog() LogView {
	return LogView{
		Runner:  "build01-1",
		Name:    "Worker_20260821-120433-utc.log",
		Size:    107374182,
		Updated: time.Date(2026, 8, 21, 12, 4, 33, 0, time.UTC),
	}
}

// どの幅でもセル数と桁が列定義に一致する（一致しないと bubbles/table の桁が崩れる）。
func TestLogRowCells(t *testing.T) {
	views := map[string]LogView{
		"標準":     sampleLog(),
		"空の値":    {},
		"長い名前":   {Runner: "build01-very-long", Name: strings.Repeat("Worker_long", 8) + ".log"},
		"巨大なサイズ": {Name: "Runner_1.log", Size: 1 << 50},
	}
	for _, width := range []int{200, 120, 80, 70, 60, 40, 20} {
		cols := molecule.Columns(token.LogColumns(), width, token.LogColumnRules())
		for name, v := range views {
			assertRowCells(t, "幅 "+strconv.Itoa(width)+" "+name, cols, LogRow(v, cols, plainStyles()))
		}
	}

	if got := LogRow(LogView{}, nil, plainStyles()); len(got) != 0 {
		t.Errorf("列が無いときのセル数 = %d, want 0", len(got))
	}
}

// 各列には runner 名・ファイル名・サイズ・更新時刻が入る。
func TestLogRowContents(t *testing.T) {
	cols := molecule.Columns(token.LogColumns(), 120, token.LogColumnRules())
	cells := LogRow(sampleLog(), cols, plainStyles())
	for i, want := range []string{"build01-1", "Worker_20260821", "102.4M", "2026-08-21 12:04"} {
		if !strings.Contains(cells[i], want) {
			t.Errorf("列 %s のセル = %q, want %q を含む", cols[i].ID, cells[i], want)
		}
	}
}

// 更新時刻が取れていない行は「値なし」の記号にする（1970 年として描かない）。
func TestLogRowWithoutUpdated(t *testing.T) {
	cols := molecule.Columns(token.LogColumns(), 120, token.LogColumnRules())
	cells := LogRow(LogView{Runner: "r", Name: "Runner_1.log", Size: 1}, cols, plainStyles())
	if !strings.Contains(cells[3], token.IconNoUnit) {
		t.Errorf("UPDATED 列 = %q, want 値なしの記号", cells[3])
	}
}

// 幅が足りないときは UPDATED → RUNNER の順に落ち、LOG と SIZE は残る（FR-23）。
func TestLogColumnsDropOrder(t *testing.T) {
	ids := func(width int) []string {
		cols := molecule.Columns(token.LogColumns(), width, token.LogColumnRules())
		out := make([]string, 0, len(cols))
		for _, c := range cols {
			out = append(out, c.ID)
		}
		return out
	}

	if got := strings.Join(ids(20), ","); got != token.ColLog+","+token.ColSize {
		t.Errorf("狭い端末の列 = %v, want LOG,SIZE", got)
	}
	if got := ids(120); len(got) != 4 {
		t.Errorf("広い端末の列 = %v, want 4 列すべて", got)
	}
}
