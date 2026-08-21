package runner

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// ProcKind は runner プロセスの種別。
type ProcKind int

const (
	ProcListener ProcKind = iota // Runner.Listener: ジョブを待ち受ける常駐プロセス
	ProcWorker                   // Runner.Worker: ジョブ 1 件ごとに起動される
)

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
}

// Elapsed は起動からの経過時間を返す。
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

	return Process{
		PID:     pid,
		Kind:    kind,
		Dir:     runnerDirFromExe(exe, procDir),
		Started: procStartTime(procDir),
		Exe:     exe,
	}, true
}

// readArgv0 は /proc/<pid>/cmdline の先頭要素を返す。cmdline は NUL 区切り。
func readArgv0(path string) string {
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

// procStartTime はプロセスの起動時刻を返す。
// /proc/<pid> ディレクトリの mtime はプロセス生成時刻になるため、
// btime + starttime/CLK_TCK を計算せずにこれを使う。
func procStartTime(procDir string) time.Time {
	fi, err := os.Stat(procDir)
	if err != nil {
		return time.Time{}
	}
	return fi.ModTime()
}
