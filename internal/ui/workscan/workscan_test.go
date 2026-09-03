package workscan_test

import (
	"os"
	"path/filepath"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/runner"
	"github.com/ousiassllc/gsr-helper/internal/ui/workscan"
)

// runnerAt は dir を runner ディレクトリとする最小の runner を返す。
func runnerAt(dir string) runner.Runner {
	return runner.Runner{Dir: dir}
}

// write は dir/name に size バイトのファイルを作る。
func write(t *testing.T, dir, name string, size int) {
	t.Helper()

	full := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(full, make([]byte, size), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

// run は Cmd を実行して Msg を取り出す。
func run(t *testing.T, cmd tea.Cmd) workscan.Msg {
	t.Helper()

	if cmd == nil {
		t.Fatal("Cmd が nil")
	}
	msg, ok := cmd().(workscan.Msg)
	if !ok {
		t.Fatal("workscan.Msg が返っていない")
	}
	return msg
}

// 複数 runner の _work を 1 つの Msg にまとめて返す。キーは runner ディレクトリ。
func TestStartCollectsEveryRunner(t *testing.T) {
	a, b := t.TempDir(), t.TempDir()
	write(t, a, "_work/repo/repo/x.bin", 50)
	write(t, b, "_work/repo/repo/y.bin", 7)

	msg := run(t, workscan.Start(3, []runner.Runner{runnerAt(a), runnerAt(b)}))

	if msg.Seq != 3 {
		t.Errorf("Seq = %d, want 3（周期の通し番号が伝わっていない）", msg.Seq)
	}
	if got := len(msg.Usage); got != 2 {
		t.Fatalf("集計した台数 = %d, want 2", got)
	}
	if got := msg.Usage[a]; got.Err != nil || got.Bytes != 50 {
		t.Errorf("a の使用量 = %+v, want 50 バイト", got)
	}
	if got := msg.Usage[b]; got.Err != nil || got.Bytes != 7 {
		t.Errorf("b の使用量 = %+v, want 7 バイト", got)
	}
}

// _work を持たない runner も 0 バイトとして載る（キーが無いのは「未集計」を表すため）。
func TestStartReportsZeroForRunnerWithoutWork(t *testing.T) {
	dir := t.TempDir()

	msg := run(t, workscan.Start(1, []runner.Runner{runnerAt(dir)}))

	got, ok := msg.Usage[dir]
	if !ok {
		t.Fatal("_work を持たない runner が結果に載っていない（未集計と区別できない）")
	}
	if got.Err != nil || got.Bytes != 0 {
		t.Errorf("使用量 = %+v, want 0 バイト・エラー無し", got)
	}
}

// runner が 1 台も無ければ Cmd を発行しない（空の周期を回さない）。
func TestStartWithoutRunnersIsNil(t *testing.T) {
	if cmd := workscan.Start(1, nil); cmd != nil {
		t.Error("runner 0 台で Cmd を発行している")
	}
}
