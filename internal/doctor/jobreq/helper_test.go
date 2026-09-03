package jobreq_test

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/doctor/check"
	"github.com/ousiassllc/gsr-helper/internal/doctor/jobreq"
	"github.com/ousiassllc/gsr-helper/internal/exec"
	"github.com/ousiassllc/gsr-helper/internal/runner"
	"github.com/ousiassllc/gsr-helper/internal/runner/procs"
)

// checkByID はこのパッケージの項目を識別子で引く。
//
// 型が非公開なので、テストからは Checks() の戻りを引くしかない。引けない
// 識別子で落とすのは、項目の名前を変えたときにテストが空振りするのを防ぐため。
func checkByID(t *testing.T, id string) check.Check {
	t.Helper()

	for _, c := range jobreq.Checks() {
		if c.ID() == id {
			return c
		}
	}
	t.Fatalf("項目 %q が Checks() に無い", id)
	return nil
}

// run は項目を 1 つ実行して結果を返す。
func run(t *testing.T, id string, in check.Input) []check.Result {
	t.Helper()
	return checkByID(t, id).Run(context.Background(), in)
}

// only は結果がちょうど 1 件であることを確かめてその 1 件を返す。
func only(t *testing.T, got []check.Result) check.Result {
	t.Helper()

	if len(got) != 1 {
		t.Fatalf("件数 = %d, want 1（%+v）", len(got), got)
	}
	return got[0]
}

// byTarget は対象 runner 名で結果を引く。
func byTarget(t *testing.T, got []check.Result, target string) check.Result {
	t.Helper()

	for _, r := range got {
		if r.Target == target {
			return r
		}
	}
	t.Fatalf("対象 %q の結果が無い（%+v）", target, got)
	return check.Result{}
}

// lookOnly は names に挙げたコマンドだけが存在する LookPath を返す。
func lookOnly(names ...string) func(string) (string, error) {
	set := make(map[string]bool, len(names))
	for _, n := range names {
		set[n] = true
	}
	return func(name string) (string, error) {
		if set[name] {
			return "/usr/bin/" + name, nil
		}
		return "", os.ErrNotExist
	}
}

// okResult は終了コード 0 と標準出力を返す。
func okResult(stdout string) exec.Result {
	return exec.Result{Stdout: []byte(stdout), Stderr: nil, ExitCode: 0}
}

// failResult は終了コードが 0 でない結果を返す。
func failResult(code int, stderr string) exec.Result {
	return exec.Result{Stdout: nil, Stderr: []byte(stderr), ExitCode: code}
}

// fakeExec は name と args から結果を決める Executor を返す。
func fakeExec(fn func(name string, args []string) (exec.Result, error)) *exec.Fake {
	f := exec.NewFake()
	f.SetFunc(fn)
	return f
}

// hostFS は /etc/group と /proc/<pid>/status を持つ差し替え用のルートを作る。
//
// procGroups は補助グループの「実体」を /proc から読む。実ホストの /proc に
// 依存させると、テストの結果が実行環境のグループ構成で変わる。
func hostFS(t *testing.T, group string, procStatus map[int]string) string {
	t.Helper()

	root := t.TempDir()
	if group != "" {
		writeFile(t, filepath.Join(root, "etc", "group"), group)
	}
	for pid, body := range procStatus {
		writeFile(t, filepath.Join(root, "proc", strconv.Itoa(pid), "status"), body)
	}
	return root
}

// writeFile は親ディレクトリごとファイルを書く。
func writeFile(t *testing.T, path, body string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

// procStatus は Groups 行を持つ /proc/<pid>/status の中身を組み立てる。
func procStatus(gids ...string) string {
	return "Name:\tRunner.Listener\nUid:\t1001\t1001\t1001\t1001\n" +
		"Groups:\t" + strings.Join(gids, " ") + "\t\n"
}

// newRunner は判定に要る欄だけを埋めた runner を返す。
func newRunner(name, user string, listenerPID int) runner.Runner {
	r := runner.Runner{
		Dir: "/opt/runners/" + name, Config: runner.Config{AgentName: name},
		Version: "", WorkDir: "", UnitName: "", RunAsUser: user,
		Managed: runner.ManagedSystemd, Svc: nil, Listener: nil, Workers: nil,
	}
	if listenerPID > 0 {
		r.Listener = &procs.Process{
			PID: listenerPID, Kind: procs.Listener, Dir: r.Dir,
			Exe: "", UID: 1001,
		}
	}
	return r
}
