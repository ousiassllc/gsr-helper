package procs

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
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
	// 稼働したまま cwd を削除されたプロセスの /proc/<pid>/cwd は
	// "<dir> (deleted)" を返す（リンク先は実在しない）。
	deletedCwd := mkDir(t, filepath.Join(base, "deleted-cwd"), nil)
	gone := filepath.Join(base, "gone")
	if err := os.Symlink(gone+" (deleted)", filepath.Join(deletedCwd, "cwd")); err != nil {
		t.Fatal(err)
	}

	tests := []struct{ name, exe, procDir, want string }{
		{"bin 配下なら 1 つ上", "/opt/r/bin/Runner.Listener", noCwd, "/opt/r"},
		{"bin 配下でなければ cwd", "/opt/r/Runner.Listener", withCwd, base},
		{"cwd も読めなければ空", "/opt/r/Runner.Listener", noCwd, ""},
		// 自動更新でバイナリが差し替わると exe に " (deleted)" が付く。
		// 印が残ると bin 配下と判定できず、ディレクトリが cwd 頼みになる。
		{"(deleted) 付きでも bin の 1 つ上", "/opt/r/bin/Runner.Listener (deleted)", noCwd, "/opt/r"},
		// cwd の印を残すと実在しないパスになり、ディレクトリが消えた runner を
		// 異常として報告することすらできなくなる。
		{"cwd の (deleted) も落とす", "/opt/r/Runner.Listener", deletedCwd, gone},
	}
	for _, tt := range tests {
		if got := runnerDirFromExe(tt.exe, tt.procDir); got != tt.want {
			t.Errorf("%s: got %q, want %q", tt.name, got, tt.want)
		}
	}
}

// runner の自動更新でバイナリが差し替わると /proc/<pid>/exe は
// "…/Runner.Listener (deleted)" を返す。印を落とさずに照合すると稼働中の runner が
// 検出から丸ごと漏れるため、両方の形を種別に落とせること。
func TestKindFromExe(t *testing.T) {
	tests := []struct {
		name, exe string
		want      Kind
		wantOK    bool
	}{
		{name: "Listener", exe: "/opt/r/bin/Runner.Listener", want: Listener, wantOK: true},
		{name: "Worker", exe: "/opt/r/bin/Runner.Worker", want: Worker, wantOK: true},
		{name: "差し替え済みの Listener", exe: "/opt/r/bin/Runner.Listener (deleted)", want: Listener, wantOK: true},
		{name: "差し替え済みの Worker", exe: "/opt/r/bin/Runner.Worker (deleted)", want: Worker, wantOK: true},
		{name: "runner ではない", exe: "/usr/bin/bash"},
		// 印だけでは runner と判定しない。"(deleted)" は落としても名前が残る。
		{name: "名前が違えば (deleted) 付きでも対象外", exe: "/opt/r/bin/Runner.Other (deleted)"},
		{name: "空", exe: ""},
	}
	for _, tt := range tests {
		got, ok := kindFromExe(tt.exe)
		if ok != tt.wantOK || (ok && got != tt.want) {
			t.Errorf("%s: kindFromExe(%q) = (%v, %v), want (%v, %v)",
				tt.name, tt.exe, got, ok, tt.want, tt.wantOK)
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

// 実 /proc を読む経路。runner が動いていないホストでも成り立つ「runner でないものを
// 弾く」ことだけを見る（検出に成功する経路は TestScanFakeProc が偽の procfs で見る）。
func TestScanOnRealProc(t *testing.T) {
	if p, ok := inspectProc(procRoot, os.Getpid()); ok {
		t.Errorf("テストプロセス自身を runner と判定した: %+v", p)
	}
	if p, ok := inspectProc(procRoot, 0); ok { // /proc/0 は存在しない（exe も cmdline も読めない）
		t.Errorf("存在しない PID を runner と判定した: %+v", p)
	}
	// procfs が読めないときはエラーを返す（0 件と区別する。Discover は警告として出す）。
	if _, err := scan(filepath.Join(t.TempDir(), "nope")); err == nil {
		t.Error("読めないルートのエラー = nil, want 非 nil")
	}
}

// 偽の procfs を組んで、検出に成功する経路が Process をどう埋めるかを見る。
// 実 /proc では Runner.Listener という名前のプロセスを起動しないとこの経路に入らない。
func TestScanFakeProc(t *testing.T) {
	root := t.TempDir()
	// 101: bin 配下の Listener。exe から種別とディレクトリの両方が決まる。
	listener := mkDir(t, filepath.Join(root, "101"),
		map[string]string{"cmdline": "/opt/r/bin/Runner.Listener\x00--startuptype\x00service\x00"})
	symlink(t, "/opt/r/bin/Runner.Listener", filepath.Join(listener, "exe"))
	// 202: exe が読めない Worker。cmdline[0] にフォールバックする。
	mkDir(t, filepath.Join(root, "202"),
		map[string]string{"cmdline": "/opt/r/bin/Runner.Worker\x00worker\x00"})
	// 303: runner ではないプロセス。走査結果に入らない。
	mkDir(t, filepath.Join(root, "303"), map[string]string{"cmdline": "/usr/bin/bash\x00"})
	// self: PID ディレクトリではない名前。数値以外は読まずに飛ばす。
	mkDir(t, filepath.Join(root, "self"),
		map[string]string{"cmdline": "/opt/r/bin/Runner.Listener\x00"})

	got, err := scan(root)
	if err != nil {
		t.Fatalf("scan() のエラー = %v", err)
	}
	want := []Process{
		{PID: 101, Kind: Listener, Dir: "/opt/r", Exe: "/opt/r/bin/Runner.Listener", UID: os.Getuid()},
		{PID: 202, Kind: Worker, Dir: "/opt/r", Exe: "/opt/r/bin/Runner.Worker", UID: os.Getuid()},
	}
	if len(got) != len(want) {
		t.Fatalf("scan() = %+v, want %d 件", got, len(want))
	}
	for i, w := range want {
		// Started は /proc/<pid> の mtime（= プロセス生成時刻）。偽の procfs では
		// ディレクトリの作成時刻になるので、同じ Stat の値と突き合わせる。
		w.Started = mtime(t, filepath.Join(root, strconv.Itoa(w.PID)))
		if got[i] != w {
			t.Errorf("scan()[%d] = %+v, want %+v", i, got[i], w)
		}
	}
}

// bin 配下でない実行ファイルのプロセスは cwd からディレクトリを導く。
// run.sh 経由など、bin 配下のパスが exe に出ない起動を落とさないため。
func TestScanFakeProcCwdFallback(t *testing.T) {
	root := t.TempDir()
	dir := mkDir(t, filepath.Join(root, "404"),
		map[string]string{"cmdline": "/opt/r/Runner.Listener\x00"})
	symlink(t, "/opt/r/work", filepath.Join(dir, "cwd"))

	got, err := scan(root)
	if err != nil {
		t.Fatalf("scan() のエラー = %v", err)
	}
	want := []Process{{
		PID: 404, Kind: Listener, Dir: "/opt/r/work",
		Exe: "/opt/r/Runner.Listener", UID: os.Getuid(), Started: mtime(t, dir),
	}}
	if len(got) != 1 || got[0] != want[0] {
		t.Errorf("scan() = %+v, want %+v", got, want)
	}
}

// symlink は target を指すリンクを作る。target は実在しなくてよい
// （/proc/<pid>/exe は差し替え済みのパスを指すことがあり、os.Readlink は追わない）。
func symlink(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("シンボリックリンクの作成に失敗しました: %v", err)
	}
}

// mtime は path の更新時刻を返す。procStat が Started に使う値と同じ。
func mtime(t *testing.T, path string) time.Time {
	t.Helper()
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat に失敗しました: %v", err)
	}
	return fi.ModTime()
}

// mkDir は dir を作り、files の各ファイル名にその内容を書く。
// runner パッケージのテストにある同名ヘルパの最小版。テスト用ヘルパのために
// 本体の識別子を公開したくないため、このパッケージで必要な分だけを持つ。
func mkDir(t *testing.T, dir string, files map[string]string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("ディレクトリの作成に失敗しました: %v", err)
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatalf("ファイルの書き込みに失敗しました: %v", err)
		}
	}
	return dir
}
