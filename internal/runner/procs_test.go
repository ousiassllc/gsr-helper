package runner

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestReadArgv0(t *testing.T) {
	// cmdline は NUL 区切りなので先頭要素だけを取り出す。
	base := mkDir(t, t.TempDir(), map[string]string{"argv": "a\x00b\x00", "plain": "abc", "empty": ""})
	tests := []struct{ name, file, want string }{
		{"NUL 区切りの先頭要素", "argv", "a"},
		{"NUL を含まない", "plain", "abc"},
		{"空ファイル", "empty", ""},
		{"存在しないパス", "nope", ""},
	}
	for _, tt := range tests {
		if got := readArgv0(filepath.Join(base, tt.file)); got != tt.want {
			t.Errorf("%s: got %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestRunnerDirFromExe(t *testing.T) {
	// procDir は /proc/<pid> の代わり。cwd シンボリックリンクの有無で分ける。
	base := t.TempDir()
	withCwd := mkDir(t, filepath.Join(base, "with-cwd"), nil)
	if err := os.Symlink(base, filepath.Join(withCwd, "cwd")); err != nil {
		t.Fatal(err)
	}
	noCwd := mkDir(t, filepath.Join(base, "no-cwd"), nil)

	tests := []struct{ name, exe, procDir, want string }{
		{"bin 配下なら 1 つ上", "/opt/r/bin/Runner.Listener", noCwd, "/opt/r"},
		{"bin 配下でなければ cwd", "/opt/r/Runner.Listener", withCwd, base},
		{"cwd も読めなければ空", "/opt/r/Runner.Listener", noCwd, ""},
	}
	for _, tt := range tests {
		if got := runnerDirFromExe(tt.exe, tt.procDir); got != tt.want {
			t.Errorf("%s: got %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestProcStat(t *testing.T) {
	// 実ディレクトリなら mtime と所有者の UID が取れる。
	if started, uid := procStat(t.TempDir()); started.IsZero() || uid != os.Getuid() {
		t.Errorf("一時ディレクトリ = (%v, %d), want (非ゼロ, %d)", started, uid, os.Getuid())
	}
	// 自プロセスの /proc/<pid> でも同じ（起動時刻と UID を 1 回の Stat から取る）。
	self := filepath.Join("/proc", strconv.Itoa(os.Getpid()))
	if started, uid := procStat(self); started.IsZero() || uid != os.Getuid() {
		t.Errorf("%s = (%v, %d), want (非ゼロ, %d)", self, started, uid, os.Getuid())
	}
	// 取得できない場合は -1。0 は root という有効値なので兼用しない。
	if started, uid := procStat(filepath.Join(t.TempDir(), "nope")); !started.IsZero() || uid != -1 {
		t.Errorf("存在しないパス = (%v, %d), want (ゼロ値, -1)", started, uid)
	}
}

// inspectProc / ScanProcesses は実 /proc を読む。検出成功の経路に入れるには
// Runner.Listener という名前のプロセスを起動する必要があり、外部コマンド実行を持ち込まない
// 方針のためここでは行わない。判定ロジックの分岐は上の 3 関数のテストで押さえ、
// ここでは「runner でないものを弾く」ことと走査自体が成功することを見る。
func TestScanProcessesOnRealProc(t *testing.T) {
	if p, ok := inspectProc(os.Getpid()); ok {
		t.Errorf("テストプロセス自身を runner と判定した: %+v", p)
	}
	if p, ok := inspectProc(0); ok { // /proc/0 は存在しない（exe も cmdline も読めない）
		t.Errorf("存在しない PID を runner と判定した: %+v", p)
	}
	// runner が動いていないホストでは 0 件になる。走査自体が成功することを見る。
	if _, err := ScanProcesses(); err != nil {
		t.Fatalf("ScanProcesses() のエラー = %v", err)
	}
}
