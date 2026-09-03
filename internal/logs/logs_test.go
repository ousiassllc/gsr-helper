package logs

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ousiassllc/gsr-helper/internal/runner"
)

// writeLog は _diag 配下にログを 1 つ作り、更新時刻を設定する。
func writeLog(t *testing.T, dir, name, body string, mod time.Time) string {
	t.Helper()

	diag := filepath.Join(dir, DiagDir)
	if err := os.MkdirAll(diag, 0o755); err != nil {
		t.Fatalf("_diag を作れない: %v", err)
	}
	path := filepath.Join(diag, name)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("%s を書けない: %v", name, err)
	}
	if err := os.Chtimes(path, mod, mod); err != nil {
		t.Fatalf("%s の時刻を設定できない: %v", name, err)
	}
	return path
}

// testRunner は dir を持つ runner を返す。
func testRunner(dir string) runner.Runner {
	r := runner.Runner{}
	r.Dir = dir
	return r
}

// 一覧は更新時刻の新しい順で、サイズを持つ（FR-23）。
func TestListOrdersByModTimeDesc(t *testing.T) {
	dir := t.TempDir()
	base := time.Date(2026, 8, 21, 12, 0, 0, 0, time.UTC)
	writeLog(t, dir, "Runner_20260821-120000-utc.log", "a\n", base)
	writeLog(t, dir, "Worker_20260821-120433-utc.log", "bb\n", base.Add(time.Minute))
	writeLog(t, dir, "Worker_20260821-115000-utc.log", "ccc\n", base.Add(-time.Hour))

	got, err := List(testRunner(dir))
	if err != nil {
		t.Fatalf("List が失敗した: %v", err)
	}
	want := []string{
		"Worker_20260821-120433-utc.log",
		"Runner_20260821-120000-utc.log",
		"Worker_20260821-115000-utc.log",
	}
	if len(got) != len(want) {
		t.Fatalf("件数 = %d, want %d（%v）", len(got), len(want), got)
	}
	for i, name := range want {
		if got[i].Name != name {
			t.Errorf("%d 番目 = %q, want %q", i, got[i].Name, name)
		}
	}
	if got[0].Size != 3 {
		t.Errorf("先頭のサイズ = %d, want 3（\"bb\\n\"）", got[0].Size)
	}
	if got[0].Kind != KindWorker {
		t.Errorf("先頭の種類 = %v, want KindWorker", got[0].Kind)
	}
}

// 更新時刻が同じログは名前の降順で並ぶ（名前に採取時刻が埋まっている）。
func TestListBreaksTieByNameDesc(t *testing.T) {
	dir := t.TempDir()
	mod := time.Date(2026, 8, 21, 12, 0, 0, 0, time.UTC)
	writeLog(t, dir, "Worker_20260821-120000-utc.log", "a\n", mod)
	writeLog(t, dir, "Worker_20260821-120001-utc.log", "a\n", mod)

	got, err := List(testRunner(dir))
	if err != nil {
		t.Fatalf("List が失敗した: %v", err)
	}
	if got[0].Name != "Worker_20260821-120001-utc.log" {
		t.Errorf("先頭 = %q, want Worker_20260821-120001-utc.log", got[0].Name)
	}
}

// `_diag` 配下の対象外のファイルは一覧に出さない。
func TestListSkipsNonLogFiles(t *testing.T) {
	dir := t.TempDir()
	mod := time.Now()
	writeLog(t, dir, "Runner_1.log", "a\n", mod)
	writeLog(t, dir, "Worker_1.json", "{}\n", mod)
	writeLog(t, dir, "SelfUpdate_1.log", "a\n", mod)
	if err := os.MkdirAll(filepath.Join(dir, DiagDir, "pages"), 0o755); err != nil {
		t.Fatalf("サブディレクトリを作れない: %v", err)
	}

	got, err := List(testRunner(dir))
	if err != nil {
		t.Fatalf("List が失敗した: %v", err)
	}
	if len(got) != 1 || got[0].Name != "Runner_1.log" {
		t.Errorf("一覧 = %v, want Runner_1.log の 1 件だけ", got)
	}
}

// `_diag` が無い runner は空を返し、エラーにしない。
func TestListWithoutDiagDirIsEmpty(t *testing.T) {
	got, err := List(testRunner(t.TempDir()))
	if err != nil {
		t.Fatalf("_diag が無いだけで失敗した: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("一覧 = %v, want 空", got)
	}
}

// 直近ジョブの Worker ログは、更新時刻が最も新しい Worker_*.log である。
func TestLatestWorkerPicksNewestWorkerLog(t *testing.T) {
	dir := t.TempDir()
	base := time.Date(2026, 8, 21, 12, 0, 0, 0, time.UTC)
	writeLog(t, dir, "Worker_old.log", "a\n", base)
	writeLog(t, dir, "Runner_new.log", "a\n", base.Add(time.Hour))
	want := writeLog(t, dir, "Worker_new.log", "a\n", base.Add(time.Minute))

	got, ok := LatestWorker(testRunner(dir))
	if !ok {
		t.Fatal("Worker ログを見つけられなかった")
	}
	if got.Path != want {
		t.Errorf("パス = %q, want %q", got.Path, want)
	}
}

// Worker ログが無い runner では偽を返す（`l` を押しても開く先が無い）。
func TestLatestWorkerWithoutWorkerLog(t *testing.T) {
	dir := t.TempDir()
	writeLog(t, dir, "Runner_1.log", "a\n", time.Now())
	if _, ok := LatestWorker(testRunner(dir)); ok {
		t.Error("Worker ログが無いのに見つかったことになっている")
	}
}
