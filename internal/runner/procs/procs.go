// Package procs は /proc を走査して稼働中の Runner.Listener / Runner.Worker を集める。
//
// runner から分離しているのは行数上限のためだけではない。/proc の走査は systemd 参照
// （internal/runner/systemd）やディスク走査と無関係な単一の情報源であり、
// 「稼働プロセスから何が分かるか」という 1 つの責務で閉じている。
// runner はこの結果とディスク・systemd の結果を突き合わせる側に専念する。
package procs

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// Kind は runner プロセスの種別。
type Kind int

// Kind の取り得る値。
const (
	Listener Kind = iota // Runner.Listener: ジョブを待ち受ける常駐プロセス
	Worker               // Runner.Worker: ジョブ 1 件ごとに起動される
)

// String は表示用の名前（プロセスの実行ファイル名）を返す。
func (k Kind) String() string {
	switch k {
	case Listener:
		return "Runner.Listener"
	case Worker:
		return "Runner.Worker"
	default:
		return "unknown"
	}
}

// deletedSuffix は実行中に実行ファイルが差し替え・削除されたプロセスの
// /proc/<pid>/exe が返す印。runner の自動更新でバイナリが置き換わると readlink は
// "<dir>/bin/Runner.Listener (deleted)" を返すため、印を落とさずに照合すると
// 稼働中の runner が検出から丸ごと漏れる。
const deletedSuffix = " (deleted)"

// execPath は照合に使う実行ファイルパスを返す。exe が差し替え済みでも
// 元のパスとして扱えるよう、末尾の " (deleted)" を落とす。
func execPath(exe string) string { return strings.TrimSuffix(exe, deletedSuffix) }

// kindFromExe は実行ファイルのパスから runner プロセスの種別を判定する。
// runner のプロセスでなければ ok が false になる。
func kindFromExe(exe string) (Kind, bool) {
	switch filepath.Base(execPath(exe)) {
	case "Runner.Listener":
		return Listener, true
	case "Runner.Worker":
		return Worker, true
	default:
		return 0, false
	}
}

// Process は検出した runner プロセス。
type Process struct {
	PID     int
	Kind    Kind
	Dir     string // 導出した runner のルートディレクトリ
	Started time.Time
	// Exe は /proc/<pid>/exe（読めなければ cmdline[0]）の生の値。バイナリが
	// 差し替えられた場合の " (deleted)" は落とさずに残す。自動更新の途中かを
	// 判断する手掛かりになるためであり、種別の判定とディレクトリの導出には
	// 印を落とした値（execPath）を使う。
	Exe string
	// UID はプロセスの実行ユーザー。取得に失敗した場合は -1。
	// 0 は root という有効値なので、失敗と兼用しない。
	UID int
}

// Elapsed は起動からの経過時間を返す。
// Jobs タブの ELAPSED 列（docs/ui/screens.md）とドレイン待機画面が Worker 単位の
// 経過時間を出すために使う。Runner.JobElapsed は最も古い Worker の値しか返さない
// ため、Worker ごとに 1 行出すこの列は作れない。
func (p Process) Elapsed() time.Duration {
	if p.Started.IsZero() {
		return 0
	}
	return time.Since(p.Started)
}

// Scan は /proc を走査して Runner.Listener / Runner.Worker を集める。
//
// 実行ファイルのパスは /proc/<pid>/exe から取るが、他ユーザーのプロセスでは
// 権限不足で読めないため cmdline[0] にフォールバックする。
func Scan() ([]Process, error) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil, fmt.Errorf("/proc の読み込みに失敗しました: %w", err)
	}

	var procs []Process
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue // 数値以外は PID ディレクトリではない
		}
		p, ok := inspectProc(pid)
		if ok {
			procs = append(procs, p)
		}
	}
	return procs, nil
}

// inspectProc は 1 プロセスを調べ、runner プロセスであれば内容を返す。
func inspectProc(pid int) (Process, bool) {
	procDir := filepath.Join("/proc", strconv.Itoa(pid))

	argv0 := readArgv0(filepath.Join(procDir, "cmdline"))
	exe, err := os.Readlink(filepath.Join(procDir, "exe"))
	if err != nil || exe == "" {
		exe = argv0
	}
	if exe == "" {
		return Process{}, false
	}

	kind, ok := kindFromExe(exe)
	if !ok {
		return Process{}, false
	}

	started, uid := procStat(procDir)
	return Process{
		PID:     pid,
		Kind:    kind,
		Dir:     runnerDirFromExe(exe, procDir),
		Started: started,
		Exe:     exe,
		UID:     uid,
	}, true
}

// readArgv0 は /proc/<pid>/cmdline の先頭要素を返す。cmdline は NUL 区切り。
func readArgv0(path string) string {
	//nolint:gosec // 呼び出し元が渡すのは /proc/<PID>/cmdline のみ。PID は strconv.Atoi で数値検証済み。
	b, err := os.ReadFile(path)
	if err != nil || len(b) == 0 {
		return ""
	}
	if i := bytes.IndexByte(b, 0); i >= 0 {
		b = b[:i]
	}
	return string(b)
}

// runnerDirFromExe は実行ファイルのパスから runner のルートディレクトリを導く。
// runner のバイナリは <runner_dir>/bin/Runner.Listener に置かれる。
// bin 配下でない場合は cwd にフォールバックする。
// exe には " (deleted)" が付いていることがある。印が付くのはパスの最終要素なので
// bin 配下かどうかの判定には影響しないが、パスとして扱う前に落としておき、
// 導出したディレクトリに印が混ざらないことをこの関数側で保証する。
func runnerDirFromExe(exe, procDir string) string {
	binDir := filepath.Dir(execPath(exe))
	if filepath.Base(binDir) == "bin" {
		return filepath.Dir(binDir)
	}
	if cwd, err := os.Readlink(filepath.Join(procDir, "cwd")); err == nil {
		return cwd
	}
	return ""
}

// procStat はプロセスの起動時刻と実行ユーザーの UID を返す。
// /proc/<pid> ディレクトリの mtime はプロセス生成時刻、所有者はプロセスの実行
// ユーザーになるため、btime + starttime/CLK_TCK の計算や status のパースをせずに
// 1 回の os.Stat で両方を取れる。同じ Stat から取ることで、PID が再利用されて
// 起動時刻と UID が別プロセスのものになることも避けられる。
// 取得できない場合はゼロ値の時刻と -1 を返す。
func procStat(procDir string) (started time.Time, uid int) {
	fi, err := os.Stat(procDir)
	if err != nil {
		return time.Time{}, -1
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return fi.ModTime(), -1
	}
	return fi.ModTime(), int(st.Uid)
}
