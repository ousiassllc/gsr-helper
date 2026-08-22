package molecule

import (
	"strings"
	"testing"
	"time"

	"github.com/ousiassllc/gsr-helper/internal/ui/token"
)

func sampleJob() JobView {
	return JobView{
		Runner:     "build01-1",
		Repository: "foo/bar",
		Elapsed:    4*time.Minute + 12*time.Second,
		WorkerPID:  284193,
		Work:       "/opt/runners/build01-1/_work/bar",
	}
}

func TestJobRowCells(t *testing.T) {
	views := map[string]JobView{
		"標準":      sampleJob(),
		"空の値":     {},
		"長い経過時間":  {Runner: "build01-1", Elapsed: 120 * time.Hour, WorkerPID: 1},
		"長いリポジトリ": {Repository: "very-long-owner-name/very-long-repository-name"},
		"長いパス":    {Runner: "build01-1", Work: strings.Repeat("/very-long-name", 8) + "/_work/bar"},
	}
	assertRows(t, token.JobColumns(), []int{200, 120, 80, 70, 60, 40}, views, JobRow)
}

// 値が取得できていない列は「値なし」の記号にする。PID は右寄せにする。
func TestJobRowContents(t *testing.T) {
	cols := Columns(token.JobColumns(), 120)
	cells := JobRow(sampleJob(), cols, plainStyles())
	for i, want := range []string{"build01-1", "foo/bar", "4m12s", "284193", "_work/bar"} {
		if !strings.Contains(cells[i], want) {
			t.Errorf("列 %s のセル = %q, want %q を含む", cols[i].ID, cells[i], want)
		}
	}
	if !strings.HasSuffix(cells[3], "284193") {
		t.Errorf("WORKER PID セル = %q, 右寄せでなければならない", cells[3])
	}

	missing := JobRow(JobView{Runner: "build01-1"}, cols, plainStyles())
	for i, name := range map[int]string{1: "REPOSITORY", 3: "WORKER PID", 4: "_work"} {
		if got := strings.TrimSpace(missing[i]); got != token.IconNoUnit {
			t.Errorf("%s セル = %q, want %q", name, got, token.IconNoUnit)
		}
	}
	if got := strings.TrimSpace(missing[2]); got != "0s" {
		t.Errorf("ELAPSED セル = %q, want %q", got, "0s")
	}
}
