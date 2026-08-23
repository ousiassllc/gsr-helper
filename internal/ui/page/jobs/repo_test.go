package jobs_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/logs"
	"github.com/ousiassllc/gsr-helper/internal/runner"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/jobs"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/pagetest"
)

// REPOSITORY / `_work` 列を Worker ログの解析で埋める（Issue #68）。
//
// 解析そのものは internal/logs のテストが見る。ここで固定するのは、
// **どのログをどのジョブに結び付けるか**と、結び付けられないときの縮退である。

// workerLog は runner の _diag に Worker ログを 1 件置く。
func workerLog(t *testing.T, dir, body string) {
	t.Helper()

	diag := filepath.Join(dir, logs.DiagDir)
	if err := os.MkdirAll(diag, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	path := filepath.Join(diag, "Worker_20260821-120544-utc.log")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

// trackingLine は tracking config の探索行（リポジトリ名の最も確実な取り出し口）。
func trackingLine(workRoot string) string {
	return "[2026-08-21 12:05:41Z INFO PipelineDirectoryManager] Loading tracking config if exists: " +
		workRoot + "/_PipelineMapping/ousiassllc/gsr-helper/PipelineFolder.json\n"
}

// jobRunner はジョブを count 件実行している runner を dir に作る。
func jobRunner(dir string, count int) runner.Runner {
	r := runner.Runner{
		Dir:     dir,
		Config:  runner.Config{AgentName: filepath.Base(dir), WorkFolder: "_work"},
		WorkDir: filepath.Join(dir, "_work"),
	}
	for i := range count {
		r.Workers = append(r.Workers, runner.Process{
			PID: 284000 + i, Kind: runner.ProcWorker, Dir: dir,
			Started: time.Now().Add(-time.Minute), UID: 1000,
		})
	}
	return r
}

// settle は共有状態を配り、解析の Cmd が返す Msg まで反映した Model を返す。
//
// **page.TabMsg を解いてから渡す。** ドメイン層の呼び出し結果は発行元のタブへ戻す
// ために包まれており（page.Do）、解くのは本番では親 Model の役目である。
func settle(t *testing.T, st page.StateMsg) tea.Model {
	t.Helper()

	m, cmd := jobs.New(1, st).Update(st)
	for _, msg := range pagetest.Msgs(cmd) {
		if tab, ok := msg.(page.TabMsg); ok {
			msg = tab.Msg
		}
		m, _ = m.Update(msg)
	}
	return m
}

// ジョブが 1 本だけの runner は Worker ログからリポジトリ名と作業ディレクトリを埋める。
func TestJobRowFillsRepositoryFromWorkerLog(t *testing.T) {
	dir := t.TempDir()
	r := jobRunner(dir, 1)
	workerLog(t, dir, trackingLine(r.WorkDir))

	body := settle(t, pagetest.State(120, 16, r)).View().Content

	// 列幅で中略されるので、先頭が一致することで見る（token.JobColumns）。
	if !strings.Contains(body, "ousiassllc/gsr-help") {
		t.Errorf("REPOSITORY 列にリポジトリ名が出ていない:\n%s", body)
	}
	// 作業ディレクトリはログに出ていないので <_work>/<repo>/<repo> を組み立てる。
	// 末尾が runner の _work ではなくリポジトリ名になっていることを見る。
	if !strings.Contains(body, "gsr-helper") {
		t.Errorf("_work 列がジョブの作業ディレクトリになっていない:\n%s", body)
	}
}

// **1 台で 2 本以上のジョブが走っている runner は引かない。**
//
// Worker ログの名前にはタイムスタンプしか無く PID が無いため、どのログがどのジョブの
// ものかを決められない。取り違えた表示は、無い表示より悪い。
func TestJobRowDoesNotGuessWhenRunnerHasSeveralJobs(t *testing.T) {
	dir := t.TempDir()
	r := jobRunner(dir, 2)
	workerLog(t, dir, trackingLine(r.WorkDir))

	body := settle(t, pagetest.State(120, 16, r)).View().Content

	if strings.Contains(body, "ousiassllc") {
		t.Errorf("同時実行中にリポジトリ名を推測している（取り違えの恐れ）:\n%s", body)
	}
}

// ログが無くても一覧は失敗せず、`_work` は runner の work ディレクトリへ縮退する。
func TestJobRowDegradesWithoutWorkerLog(t *testing.T) {
	dir := t.TempDir()
	r := jobRunner(dir, 1)

	body := settle(t, pagetest.State(120, 16, r)).View().Content

	if body == "" {
		t.Fatal("ログが無いだけで一覧が空になっている")
	}
	if strings.Contains(body, "ousiassllc") {
		t.Errorf("読めていないのにリポジトリ名が出ている:\n%s", body)
	}
}

// 形式が想定外のログでも縮退するだけで壊れない。
func TestJobRowDegradesOnUnknownLogFormat(t *testing.T) {
	dir := t.TempDir()
	r := jobRunner(dir, 1)
	workerLog(t, dir, "[2026-08-21 14:00:01Z ERR  Worker] Job canceled before checkout started.\n")

	body := settle(t, pagetest.State(120, 16, r)).View().Content

	if body == "" {
		t.Fatal("解析できないログで一覧が空になっている")
	}
	if strings.Contains(body, "ousiassllc") {
		t.Errorf("解析できていないのにリポジトリ名が出ている:\n%s", body)
	}
}
