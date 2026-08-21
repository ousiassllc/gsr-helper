package runner

import (
	"fmt"
	"testing"
	"time"

	"github.com/ousiassllc/gsr-helper/internal/runner/scope"
)

func TestRunnerName(t *testing.T) {
	// AgentName があればそれ、無ければディレクトリ名（末尾スラッシュは落とす）。
	tests := []struct {
		runner Runner
		want   string
	}{
		{Runner{Dir: "/opt/r1", Config: Config{AgentName: "host-1"}}, "host-1"},
		{Runner{Dir: "/opt/actions-runner-2"}, "actions-runner-2"},
		{Runner{Dir: "/opt/actions-runner-3/"}, "actions-runner-3"},
	}
	for _, tt := range tests {
		if got := tt.runner.Name(); got != tt.want {
			t.Errorf("Name() = %q, want %q", got, tt.want)
		}
	}
}

func TestRunnerRunningAndBusy(t *testing.T) {
	lis := &Process{PID: 1, Kind: ProcListener}
	tests := []struct {
		name          string
		runner        Runner
		running, busy bool
	}{
		{"停止・アイドル", Runner{}, false, false},
		{"Workers が空スライス", Runner{Workers: []Process{}}, false, false},
		{"稼働中・アイドル", Runner{Listener: lis}, true, false},
		{"稼働中・ジョブ 1 件", Runner{Listener: lis, Workers: []Process{{PID: 2}}}, true, true},
		{"ジョブ複数", Runner{Workers: []Process{{PID: 2}, {PID: 3}}}, false, true},
	}
	for _, tt := range tests {
		if r, b := tt.runner.Running(), tt.runner.Busy(); r != tt.running || b != tt.busy {
			t.Errorf("%s: (Running, Busy) = (%v, %v), want (%v, %v)", tt.name, r, b, tt.running, tt.busy)
		}
	}
}

func TestLongestElapsed(t *testing.T) {
	now := time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)
	at := func(d time.Duration) Process { return Process{Started: now.Add(-d)} }

	tests := []struct {
		name  string
		procs []Process
		want  time.Duration
	}{
		{"0 件", nil, 0},
		{"1 件", []Process{at(90 * time.Second)}, 90 * time.Second},
		{"複数なら最大", []Process{at(time.Minute), at(10 * time.Minute), at(5 * time.Minute)}, 10 * time.Minute},
		{"Started がゼロ値は無視", []Process{{}, at(time.Minute)}, time.Minute},
		{"全てゼロ値", []Process{{}, {}}, 0},
		{"未来の Started でも負にならない", []Process{at(-time.Hour)}, 0},
	}
	for _, tt := range tests {
		if got := longestElapsed(tt.procs, now); got != tt.want {
			t.Errorf("%s: got %v, want %v", tt.name, got, tt.want)
		}
	}
}

// JobElapsed と Process.Elapsed は longestElapsed / time.Since の薄いラッパ。
// 境界値は TestLongestElapsed で見ているので、ここでは実時間で概算だけ確かめる。
func TestElapsed(t *testing.T) {
	r := Runner{Workers: []Process{
		{PID: 1, Started: time.Now().Add(-30 * time.Second)},
		{PID: 2, Started: time.Now().Add(-2 * time.Minute)},
	}}
	if got := r.JobElapsed(); got < 2*time.Minute || got > 3*time.Minute {
		t.Errorf("JobElapsed() = %v, want 約 2 分（最も古い Worker）", got)
	}
	if got := r.Workers[0].Elapsed(); got < 30*time.Second || got > time.Minute {
		t.Errorf("Elapsed() = %v, want 約 30 秒（Worker 単位）", got)
	}
	if a, b := (Runner{}).JobElapsed(), (Process{}).Elapsed(); a != 0 || b != 0 {
		t.Errorf("Worker なし / Started がゼロ値で (%v, %v), want (0, 0)", a, b)
	}
}

func TestSortRunners(t *testing.T) {
	mk := func(sc scope.Scope, name string) Runner {
		return Runner{Dir: "/opt/" + name, Scope: sc, Config: Config{AgentName: name}}
	}
	org := scope.Scope{Kind: scope.Org, Owner: "myorg"}
	repo := scope.Scope{Kind: scope.Repo, Owner: "myorg", Repo: "myrepo"}

	// Unknown のスコープ表記は "-" なので "myorg/myrepo" / "org:myorg" より前に来る。
	// 同名・同スコープ（"a" が 2 件）でも件数が変わらないことを併せて確かめる。
	runners := []Runner{
		mk(org, "b"), mk(scope.Scope{}, "-unknown"), mk(repo, "z"), mk(org, "a"), mk(repo, "a"), mk(org, "a"),
	}
	sortRunners(runners)

	got := make([]string, 0, len(runners))
	for _, r := range runners {
		got = append(got, r.Scope.String()+"/"+r.Name())
	}
	want := []string{"-/-unknown", "myorg/myrepo/a", "myorg/myrepo/z", "org:myorg/a", "org:myorg/a", "org:myorg/b"}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("runners[%d] = %q, want %q（全体: %q）", i, got[i], w, got)
		}
	}
}

// ManagedBy / ProcKind の String は表示用の写像。範囲外の値でも既定値を返すこと。
func TestKindStrings(t *testing.T) {
	tests := []struct {
		in   fmt.Stringer
		want string
	}{
		{ManagedSystemd, "systemd"}, {ManagedStandalone, "run.sh"},
		{ManagedUnknown, "-"}, {ManagedBy(99), "-"},
		{ProcListener, "Runner.Listener"}, {ProcWorker, "Runner.Worker"}, {ProcKind(99), "unknown"},
	}
	for _, tt := range tests {
		if got := tt.in.String(); got != tt.want {
			t.Errorf("%T の String() = %q, want %q", tt.in, got, tt.want)
		}
	}
}
