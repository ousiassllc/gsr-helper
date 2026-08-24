package filerow_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	dlogs "github.com/ousiassllc/gsr-helper/internal/logs"
	"github.com/ousiassllc/gsr-helper/internal/runner"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/logs/filerow"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/pagetest"
)

// tea.Model を組み立てずに検証できるのがこのパッケージを分けた理由なので、
// ここでは runner とログファイルを入力に、一覧の並びと絞り込みだけを見る。

// withLogs は `_diag` にログを持つ runner を返す。mod は名前ごとの更新時刻。
func withLogs(t *testing.T, name string, mod map[string]time.Time) runner.Runner {
	t.Helper()

	dir := filepath.Join(t.TempDir(), name)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("runner ディレクトリを作れない: %v", err)
	}
	r := pagetest.SampleRunner()
	r.Dir = dir
	r.Config.AgentName = name

	for file, at := range mod {
		if _, err := pagetest.WriteDiagLog(dir, file, "line\n", at); err != nil {
			t.Fatalf("ログを用意できない: %v", err)
		}
	}
	return r
}

// 一覧は runner をまたいで更新時刻の降順に並ぶ（FR-23）。
//
// **runner ごとに区切らないことが要件である。** 区切ると、どの区画の先頭が最新かを
// 目で追うことになる。利用者が見たいのは「直近に動いたログ」である。
func TestRowsSortsAcrossRunnersByNewest(t *testing.T) {
	t.Parallel()

	base := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	a := withLogs(t, "build01-1", map[string]time.Time{"Worker_old.log": base.Add(-2 * time.Hour)})
	b := withLogs(t, "build01-2", map[string]time.Time{"Worker_new.log": base})

	rows := filerow.Rows([]runner.Runner{a, b})
	if len(rows) != 2 {
		t.Fatalf("行数 = %d, want 2", len(rows))
	}
	if rows[0].File.Name != "Worker_new.log" {
		t.Errorf("先頭 = %q, want Worker_new.log（新しい方が先）", rows[0].File.Name)
	}
	if rows[0].Runner.Name() == rows[1].Runner.Name() {
		t.Error("2 台ぶんの行が 1 台に潰れている")
	}
}

// 同時刻はファイル名の降順で並ぶ（並びが実行ごとに揺れないこと）。
func TestRowsBreaksTiesByNameDescending(t *testing.T) {
	t.Parallel()

	at := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	r := withLogs(t, "build01-1", map[string]time.Time{"Worker_1.log": at, "Worker_2.log": at})

	rows := filerow.Rows([]runner.Runner{r})
	if len(rows) != 2 {
		t.Fatalf("行数 = %d, want 2", len(rows))
	}
	if rows[0].File.Name != "Worker_2.log" || rows[1].File.Name != "Worker_1.log" {
		t.Errorf("並び = %q, %q, want Worker_2.log, Worker_1.log", rows[0].File.Name, rows[1].File.Name)
	}
}

// `_diag` を読めない runner は一覧から落ちるだけで、他の runner を巻き添えにしない。
func TestRowsSkipsUnreadableRunner(t *testing.T) {
	t.Parallel()

	at := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	ok := withLogs(t, "build01-1", map[string]time.Time{"Worker_1.log": at})
	bad := pagetest.SampleRunner()
	bad.Dir = filepath.Join(t.TempDir(), "無い")

	rows := filerow.Rows([]runner.Runner{bad, ok})
	if len(rows) != 1 {
		t.Fatalf("行数 = %d, want 1（読めない runner の行だけが落ちる）", len(rows))
	}
	if rows[0].File.Name != "Worker_1.log" {
		t.Errorf("残った行 = %q, want Worker_1.log", rows[0].File.Name)
	}
}

// 表は 1 区画だけを持ち、入れた行がそのままカーソルの対象になる。
//
// Logs タブに一括操作は無く、対象は常にカーソル位置の 1 件である（Jobs タブと
// 同じ理由）。区画の添字を tab 側と共有するために Section を公開している。
func TestNewTableShowsRowsInTheOnlySection(t *testing.T) {
	t.Parallel()

	tbl := filerow.NewTable(pagetest.Keys(), pagetest.Styles())
	want := filerow.Row{File: dlogs.File{Name: "Worker_1.log", Path: "/tmp/Worker_1.log"}}
	tbl.SetItems(filerow.Section, []filerow.Row{want})

	if got := len(tbl.Shown(filerow.Section)); got != 1 {
		t.Fatalf("表示している行 = %d, want 1", got)
	}
	got, ok := tbl.Selected()
	if !ok {
		t.Fatal("カーソルがどの行も指していない")
	}
	if got.File.Path != want.File.Path {
		t.Errorf("カーソル位置の行 = %q, want %q", got.File.Path, want.File.Path)
	}
}
