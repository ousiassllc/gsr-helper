package disk

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/ousiassllc/gsr-helper/internal/runner"
	"github.com/ousiassllc/gsr-helper/internal/runner/procs"
	"github.com/ousiassllc/gsr-helper/internal/runner/scope"
	"github.com/ousiassllc/gsr-helper/internal/runner/systemd"
)

const testRunnerName = "build01-1"

// newRunner はテスト用の runner.Runner を組み立てる。busy が真なら Worker を 1 つ
// 持たせ、ジョブ実行中（FR-31）の runner にする。
func newRunner(dir string, busy bool) runner.Runner {
	var workers []procs.Process
	if busy {
		workers = []procs.Process{{
			PID: 1234, Kind: procs.Worker, Dir: dir,
			Started: time.Now(), Exe: filepath.Join(dir, "bin", "Runner.Worker"), UID: 0,
		}}
	}
	return runner.Runner{
		Dir: dir,
		Config: runner.Config{
			AgentID: 1, AgentName: testRunnerName, PoolID: 0, PoolName: "",
			ServerURL: "", GitHubURL: "https://github.com/myorg/myrepo",
			WorkFolder: "_work", Ephemeral: false, DisableUpdate: true,
		},
		Scope:     scope.Scope{Kind: scope.Repo, Owner: "myorg", Repo: "myrepo"},
		Version:   "2.309.0",
		WorkDir:   filepath.Join(dir, "_work"),
		UnitName:  "",
		RunAsUser: "runner",
		Managed:   runner.ManagedStandalone,
		Svc:       (*systemd.State)(nil),
		Listener:  (*procs.Process)(nil),
		Workers:   workers,
	}
}

// newScanTree は集計対象の入った runner ディレクトリを作る。
//
//	_work/repo   : a.txt(100) + sub/b.txt(50) + link(シンボリックリンク)
//	_work/_tool  : t.txt(10)
//	_work/_temp  : 空
//	_work/stray.txt : ディレクトリではないので対象外
//	_diag        : d.log(7)
func newScanTree(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	repo := filepath.Join(dir, "_work", "repo")
	mkFile(t, filepath.Join(repo, "a.txt"), 100)
	mkFile(t, filepath.Join(repo, "sub", "b.txt"), 50)
	if err := os.Symlink(filepath.Join(repo, "a.txt"), filepath.Join(repo, "link")); err != nil {
		t.Fatalf("シンボリックリンクの作成に失敗した: %v", err)
	}
	mkFile(t, filepath.Join(dir, "_work", "_tool", "t.txt"), 10)
	if err := os.MkdirAll(filepath.Join(dir, "_work", "_temp"), 0o755); err != nil {
		t.Fatalf("_temp の作成に失敗した: %v", err)
	}
	mkFile(t, filepath.Join(dir, "_work", "stray.txt"), 3)
	mkFile(t, filepath.Join(dir, "_diag", "d.log"), 7)
	return dir
}

// mkFile は size バイトのファイルを作る。親ディレクトリも作る。
func mkFile(t *testing.T, path string, size int) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("%s の作成に失敗した: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, make([]byte, size), 0o600); err != nil {
		t.Fatalf("%s の作成に失敗した: %v", path, err)
	}
}

// collect は Scan の送出を全件受け取り、Label 順に並べて返す。
// Scan は out を閉じないため、閉じるのは呼び出し側（ここ）の責務である。
func collect(ctx context.Context, r runner.Runner) []Usage {
	out := make(chan Usage)
	go func() {
		Scan(ctx, r, out)
		close(out)
	}()

	var got []Usage
	for u := range out {
		got = append(got, u)
	}
	sort.Slice(got, func(i, j int) bool { return got[i].Label < got[j].Label })
	return got
}

func TestScan(t *testing.T) {
	dir := newScanTree(t)
	got := collect(context.Background(), newRunner(dir, false))

	// シンボリックリンクは 1 ファイルと数えるがサイズは 0。_work 直下の
	// ファイル（stray.txt）は内訳の単位にならないので現れない。
	want := []Usage{
		{Kind: KindDiag, Label: testRunnerName + " / _diag", Path: filepath.Join(dir, "_diag"), Bytes: 7, Files: 1},
		{Kind: KindTemp, Label: testRunnerName + " / _work/_temp", Path: filepath.Join(dir, "_work", "_temp"), Bytes: 0, Files: 0},
		{Kind: KindTool, Label: testRunnerName + " / _work/_tool", Path: filepath.Join(dir, "_work", "_tool"), Bytes: 10, Files: 1},
		{Kind: KindWork, Label: testRunnerName + " / _work/repo", Path: filepath.Join(dir, "_work", "repo"), Bytes: 150, Files: 3},
	}
	if len(got) != len(want) {
		t.Fatalf("送出件数 = %d, want %d（%+v）", len(got), len(want), got)
	}
	for i, w := range want {
		g := got[i]
		if g.Kind != w.Kind || g.Label != w.Label || g.Path != w.Path || g.Bytes != w.Bytes || g.Files != w.Files {
			t.Errorf("Usage[%d] = %+v, want Kind=%v Label=%q Path=%q Bytes=%d Files=%d",
				i, g, w.Kind, w.Label, w.Path, w.Bytes, w.Files)
		}
		if g.Err != nil {
			t.Errorf("Usage[%d].Err = %v, want nil", i, g.Err)
		}
		if g.Runner != testRunnerName || g.Base != dir {
			t.Errorf("Usage[%d] Runner=%q Base=%q, want %q %q", i, g.Runner, g.Base, testRunnerName, dir)
		}
		if !g.Removable || g.Reason != "" {
			t.Errorf("Usage[%d] Removable=%v Reason=%q, want true と空", i, g.Removable, g.Reason)
		}
	}
}

// TestScanBusyRunner は実行中ジョブの保護（FR-31）を確かめる。_work 配下だけを
// 選べなくし、_diag は実行中でも選べる。
func TestScanBusyRunner(t *testing.T) {
	dir := newScanTree(t)
	got := collect(context.Background(), newRunner(dir, true))

	for _, u := range got {
		wantRemovable := u.Kind == KindDiag
		if u.Removable != wantRemovable {
			t.Errorf("%s の Removable = %v, want %v", u.Label, u.Removable, wantRemovable)
		}
		wantReason := ""
		if !wantRemovable {
			wantReason = busyReason
		}
		if u.Reason != wantReason {
			t.Errorf("%s の Reason = %q, want %q", u.Label, u.Reason, wantReason)
		}
	}
}

// TestScanWithoutWork は _work を持たない runner がエラー行を並べないことを確かめる。
// 起動直後でジョブを 1 度も実行していない runner は正常な状態である。
func TestScanWithoutWork(t *testing.T) {
	dir := t.TempDir()
	if got := collect(context.Background(), newRunner(dir, false)); len(got) != 0 {
		t.Errorf("送出 = %+v, want 0 件", got)
	}
}

// TestScanCanceled はキャンセル済みの ctx で送出を待ち続けないことを確かめる。
// out に読み手が居なくても Scan が返らなければ、UI の終了が止まる。
func TestScanCanceled(t *testing.T) {
	dir := newScanTree(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		Scan(ctx, newRunner(dir, false), make(chan Usage)) // 読み手の居ない channel
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("キャンセル済みの ctx で Scan が返らなかった")
	}
}
