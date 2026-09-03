package hostcfg_test

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/doctor/check"
	"github.com/ousiassllc/gsr-helper/internal/doctor/hostcfg"
	"github.com/ousiassllc/gsr-helper/internal/exec"
	"github.com/ousiassllc/gsr-helper/internal/runner"
	"github.com/ousiassllc/gsr-helper/internal/runner/systemd"
)

func checkByID(t *testing.T, id string) check.Check {
	t.Helper()

	for _, c := range hostcfg.Checks() {
		if c.ID() == id {
			return c
		}
	}
	t.Fatalf("項目 %q が Checks() に無い", id)
	return nil
}

func run(t *testing.T, id string, in check.Input) []check.Result {
	t.Helper()
	return checkByID(t, id).Run(context.Background(), in)
}

func only(t *testing.T, got []check.Result) check.Result {
	t.Helper()

	if len(got) != 1 {
		t.Fatalf("件数 = %d, want 1（%+v）", len(got), got)
	}
	return got[0]
}

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

func okResult(stdout string) exec.Result {
	return exec.Result{Stdout: []byte(stdout), Stderr: nil, ExitCode: 0}
}

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

// unitFixture は systemctl show が返すユニット 1 つぶんの値。
type unitFixture struct {
	load       string
	active     string
	workingDir string
	restart    string
	env        string
}

// systemctlFake は list-units と show に応える Executor を返す。
//
// systemd.Scan は list-units → ユニットごとの show を発行する。孤児と重複の
// 判定はその戻りに乗るので、実ホストの systemd に依存させずに検査できる。
func systemctlFake(units map[string]unitFixture, order []string) *exec.Fake {
	f := exec.NewFake()
	f.SetFunc(func(_ string, args []string) (exec.Result, error) {
		if len(args) > 0 && args[0] == "list-units" {
			return okResult(strings.Join(order, "\n") + "\n"), nil
		}
		if len(args) > 1 && args[0] == "show" {
			u, ok := units[args[1]]
			if !ok {
				return okResult(""), nil
			}
			return okResult(
				"Id=" + args[1] + "\n" +
					"LoadState=" + u.load + "\n" +
					"ActiveState=" + u.active + "\n" +
					"SubState=running\n" +
					"UnitFileState=enabled\n" +
					"WorkingDirectory=" + u.workingDir + "\n" +
					"MainPID=100\n" +
					"User=runner\n" +
					"Restart=" + u.restart + "\n" +
					"Environment=" + u.env + "\n",
			), nil
		}
		return okResult(""), nil
	})
	return f
}

func newRunner(name, dir, unit string) runner.Runner {
	r := runner.Runner{
		Dir:      dir,
		Config:   runner.Config{AgentName: name},
		UnitName: unit,
		Managed:  runner.ManagedSystemd,
	}
	if unit != "" {
		r.Svc = &systemd.State{Unit: unit, Load: "loaded", Active: "active", WorkingDir: dir}
	}
	return r
}

// systemctlListUnitsFails は list-units だけが失敗する Executor を返す。
//
// systemctl は在るのに一覧が取れない状態（コンテナ内・dbus 停止）を再現する。
// 実運用で到達する状態であり、ここで 0 件と区別できないと「検査していないのに
// OK」を返してしまう。
func systemctlListUnitsFails() *exec.Fake {
	f := exec.NewFake()
	f.SetFunc(func(_ string, args []string) (exec.Result, error) {
		if len(args) > 0 && args[0] == "list-units" {
			return exec.Result{
				Stdout:   nil,
				Stderr:   []byte("Failed to connect to bus: No such file or directory"),
				ExitCode: 1,
			}, nil
		}
		return okResult(""), nil
	})
	return f
}
