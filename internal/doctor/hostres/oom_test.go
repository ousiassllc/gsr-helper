package hostres_test

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ousiassllc/gsr-helper/internal/appconfig"
	"github.com/ousiassllc/gsr-helper/internal/doctor/check"
	"github.com/ousiassllc/gsr-helper/internal/exec"
	"github.com/ousiassllc/gsr-helper/internal/runner"
)

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
