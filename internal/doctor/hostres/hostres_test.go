package hostres_test

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ousiassllc/gsr-helper/internal/appconfig"
	"github.com/ousiassllc/gsr-helper/internal/doctor/check"
	"github.com/ousiassllc/gsr-helper/internal/doctor/hostres"
	"github.com/ousiassllc/gsr-helper/internal/exec"
	"github.com/ousiassllc/gsr-helper/internal/runner"
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

func TestNTP(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		stdout string
		look   func(string) (string, error)
		want   check.Status
	}{
		"同期済み": {
			stdout: "NTP=yes\nNTPSynchronized=yes\nTimeUSec=Sat 2026-08-23 12:00:00 JST\n",
			look:   lookOnly("timedatectl"), want: check.OK,
		},
		"未同期": {
			stdout: "NTP=yes\nNTPSynchronized=no\nTimeUSec=Sat 2026-08-23 12:00:00 JST\n",
			look:   lookOnly("timedatectl"), want: check.Fail,
		},
		"NTP そのものが無効": {
			stdout: "NTP=no\nNTPSynchronized=no\nTimeUSec=Sat 2026-08-23 12:00:00 JST\n",
			look:   lookOnly("timedatectl"), want: check.Fail,
		},
		"timedatectl が無い": {
			stdout: "", look: lookOnly(), want: check.Skip,
		},
		"出力に NTPSynchronized が無い": {
			stdout: "TimeUSec=Sat 2026-08-23 12:00:00 JST\n",
			look:   lookOnly("timedatectl"), want: check.Skip,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			in := check.Input{
				Exec:     fakeExec(func(string, []string) (exec.Result, error) { return okResult(tt.stdout), nil }),
				LookPath: tt.look,
			}
			got := only(t, run(t, "time.ntp", in))
			if got.Status != tt.want {
				t.Errorf("Status = %v, want %v（Detail: %s）", got.Status, tt.want, got.Detail)
			}
		})
	}
}

// ずれの秒数は外部の基準時刻が要るため出さない。出さない理由を Detail に書く。
func TestNTPDoesNotClaimDriftSeconds(t *testing.T) {
	t.Parallel()

	in := check.Input{
		Exec: fakeExec(func(string, []string) (exec.Result, error) {
			return okResult("NTP=yes\nNTPSynchronized=no\nTimeUSec=x\n"), nil
		}),
		LookPath: lookOnly("timedatectl"),
	}
	got := only(t, run(t, "time.ntp", in))
	if !strings.Contains(got.Detail, "ずれの秒数") {
		t.Errorf("ずれを測っていない旨が Detail に無い: %s", got.Detail)
	}
	if !strings.Contains(got.Remedy, "timedatectl set-ntp true") {
		t.Errorf("Remedy に同期を有効にする手順が無い: %s", got.Remedy)
	}
}

func TestMemory(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		meminfo string
		want    check.Status
	}{
		"余裕がある": {
			meminfo: "MemTotal: 1000 kB\nMemAvailable: 800 kB\nSwapTotal: 500 kB\nSwapFree: 500 kB\n",
			want:    check.OK,
		},
		"空きが 20% を切る": {
			meminfo: "MemTotal: 1000 kB\nMemAvailable: 150 kB\nSwapTotal: 500 kB\nSwapFree: 500 kB\n",
			want:    check.Warn,
		},
		"空きが 10% を切る": {
			meminfo: "MemTotal: 1000 kB\nMemAvailable: 50 kB\nSwapTotal: 500 kB\nSwapFree: 500 kB\n",
			want:    check.Fail,
		},
		"余裕はあるが swap が無い": {
			meminfo: "MemTotal: 1000 kB\nMemAvailable: 800 kB\nSwapTotal: 0 kB\nSwapFree: 0 kB\n",
			want:    check.Warn,
		},
		"必要な項目が無い": {
			meminfo: "MemTotal: 1000 kB\n",
			want:    check.Skip,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			in := check.Input{FSRoot: procFS(t, "meminfo", tt.meminfo)}
			got := only(t, run(t, "resource.mem", in))
			if got.Status != tt.want {
				t.Errorf("Status = %v, want %v（Detail: %s）", got.Status, tt.want, got.Detail)
			}
		})
	}
}

// /proc/meminfo を読めなければ SKIP。
func TestMemoryWithoutMeminfo(t *testing.T) {
	t.Parallel()

	if got := only(t, run(t, "resource.mem", check.Input{FSRoot: t.TempDir()})).Status; got != check.Skip {
		t.Errorf("Status = %v, want %v", got, check.Skip)
	}
}

// 遡る幅は現在時刻から決める。テストで固定できることが要件である。
func TestOOMPassesSinceFromClock(t *testing.T) {
	t.Parallel()

	fixed := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
	f := fakeExec(func(string, []string) (exec.Result, error) { return okResult(""), nil })
	in := check.Input{
		Exec:     f,
		LookPath: lookOnly("journalctl"),
		Caps:     appconfig.Caps{Journal: true},
		Now:      func() time.Time { return fixed },
	}

	if got := only(t, run(t, "history.oom", in)).Status; got != check.OK {
		t.Errorf("Status = %v, want %v", got, check.OK)
	}

	calls := f.Calls()
	if len(calls) != 1 {
		t.Fatalf("呼び出し回数 = %d, want 1", len(calls))
	}
	want := []string{"-k", "--since", "2026-08-22 12:00:00", "--no-pager"}
	if !slices.Equal(calls[0].Args, want) {
		t.Errorf("引数 = %q, want %q", calls[0].Args, want)
	}
	// 診断のコマンドは監査ログに記録する（押し流しが起きないため）。
	if calls[0].Options.SkipAudit {
		t.Error("SkipAudit = true, want false")
	}
}

// OOM の痕跡は runner ごとにまとめる。どの runner が落ちているかが対処を決める。
func TestOOMAttributesToRunner(t *testing.T) {
	t.Parallel()

	const kernelLog = "Aug 23 10:00:00 host kernel: Out of memory: Killed process 111 (build01-worker)\n" +
		"Aug 23 10:05:00 host kernel: oom-kill: constraint=CONSTRAINT_NONE\n" +
		"Aug 23 10:06:00 host kernel: Out of memory: Killed process 222 (build01-worker)\n" +
		"Aug 23 11:00:00 host kernel: usb 1-1: new high-speed USB device\n"

	in := check.Input{
		Exec:     fakeExec(func(string, []string) (exec.Result, error) { return okResult(kernelLog), nil }),
		LookPath: lookOnly("journalctl"),
		Caps:     appconfig.Caps{Journal: true},
		Runners: []runner.Runner{
			{Dir: "/opt/runners/build01", Config: runner.Config{AgentName: "build01"}},
		},
	}

	got := run(t, "history.oom", in)
	if len(got) != 2 {
		t.Fatalf("件数 = %d, want 2（build01 の行とホスト全体の行）: %+v", len(got), got)
	}
	if got[0].Target != "build01" {
		t.Errorf("1 行目の Target = %q, want %q", got[0].Target, "build01")
	}
	if got[0].Status != check.Warn {
		t.Errorf("Status = %v, want %v（履歴であって現在の障害ではない）", got[0].Status, check.Warn)
	}
	if !strings.Contains(got[0].Detail, "2 件") {
		t.Errorf("件数が Detail に無い: %s", got[0].Detail)
	}
	if got[1].Target != "" {
		t.Errorf("2 行目の Target = %q, want 空（ホスト全体）", got[1].Target)
	}
}

// journalctl が無ければ SKIP。
func TestOOMWithoutJournal(t *testing.T) {
	t.Parallel()

	in := check.Input{
		Exec:     exec.NewFake(),
		LookPath: lookOnly(),
		Caps:     appconfig.Caps{Journal: false},
	}
	if got := only(t, run(t, "history.oom", in)).Status; got != check.Skip {
		t.Errorf("Status = %v, want %v", got, check.Skip)
	}
}
