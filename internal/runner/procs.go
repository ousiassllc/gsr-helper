package runner

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"time"
)

// ProcKind は runner プロセスの種別。
type ProcKind int

// ProcKind の取り得る値。
const (
	ProcListener ProcKind = iota // Runner.Listener: ジョブを待ち受ける常駐プロセス
	ProcWorker                   // Runner.Worker: ジョブ 1 件ごとに起動される
)

// String は表示用の名前（プロセスの実行ファイル名）を返す。
func (k ProcKind) String() string {
	switch k {
	case ProcListener:
		return "Runner.Listener"
	case ProcWorker:
		return "Runner.Worker"
	default:
		return "unknown"
	}
}

// Process は検出した runner プロセス。
type Process struct {
	PID     int
	Kind    ProcKind
	Dir     string // 導出した runner のルートディレクトリ
	Started time.Time
	Exe     string
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

// ScanProcesses は /proc を走査して Runner.Listener / Runner.Worker を集める。
//
// 実行ファイルのパスは /proc/<pid>/exe から取るが、他ユーザーのプロセスでは
// 権限不足で読めないため cmdline[0] にフォールバックする。
func ScanProcesses() ([]Process, error) {
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

	var kind ProcKind
	switch filepath.Base(exe) {
	case "Runner.Listener":
		kind = ProcListener
	case "Runner.Worker":
		kind = ProcWorker
	default:
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
func runnerDirFromExe(exe, procDir string) string {
	binDir := filepath.Dir(exe)
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
