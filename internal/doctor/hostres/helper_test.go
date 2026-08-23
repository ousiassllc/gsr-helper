package hostres_test

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/doctor/check"
	"github.com/ousiassllc/gsr-helper/internal/doctor/hostres"
	"github.com/ousiassllc/gsr-helper/internal/exec"
)

func checkByID(t *testing.T, id string) check.Check {
	t.Helper()

	for _, c := range hostres.Checks() {
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

func fakeExec(fn func(name string, args []string) (exec.Result, error)) *exec.Fake {
	f := exec.NewFake()
	f.SetFunc(fn)
	return f
}

func okResult(stdout string) exec.Result {
	return exec.Result{Stdout: []byte(stdout), Stderr: nil, ExitCode: 0}
}

// procFS は /proc 配下のファイルを持つ差し替え用のルートを作る。
func procFS(t *testing.T, name, body string) string {
	t.Helper()

	root := t.TempDir()
	path := filepath.Join(root, "proc", name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return root
}

// この 3 分類は起動時の自動判定（FR-44）に入れない。
// 起動のたびに statfs と journalctl を走らせると runner 一覧が出るまでが延びる。
func TestHostResChecksAreNotRunAtStartup(t *testing.T) {
	t.Parallel()

	want := map[string]string{
		"time.ntp":     check.CatTime,
		"resource.fs":  check.CatResource,
		"resource.mem": check.CatResource,
		"history.oom":  check.CatHistory,
	}
	var ids []string
	for _, c := range hostres.Checks() {
		ids = append(ids, c.ID())
		if c.Startup() {
			t.Errorf("%s が起動時の対象になっている", c.ID())
		}
		if got := c.Category(); got != want[c.ID()] {
			t.Errorf("%s の Category = %q, want %q", c.ID(), got, want[c.ID()])
		}
	}
	slices.Sort(ids)
	wantIDs := []string{"history.oom", "resource.fs", "resource.mem", "time.ntp"}
	if !slices.Equal(ids, wantIDs) {
		t.Errorf("項目 = %q, want %q", ids, wantIDs)
	}
}
