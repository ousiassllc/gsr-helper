package hostreq_test

import (
	"context"
	"sync/atomic"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/doctor"
	"github.com/ousiassllc/gsr-helper/internal/doctor/check"
	"github.com/ousiassllc/gsr-helper/internal/ui/hostreq"
)

// stub は起動時の前提チェックの差し替え。実ホストを見ない。
type stub struct {
	id     string
	status check.Status
	runs   *atomic.Int32
}

func (s stub) ID() string     { return s.id }
func (stub) Category() string { return check.CatJobReq }
func (stub) Startup() bool    { return true }

func (s stub) Run(context.Context, check.Input) []check.Result {
	if s.runs != nil {
		s.runs.Add(1)
	}
	return []check.Result{{ID: s.id, Category: check.CatJobReq, Status: s.status}}
}

// run は Cmd を実行して結果を返す。
func run(t *testing.T, cmd tea.Cmd) hostreq.Msg {
	t.Helper()

	if cmd == nil {
		t.Fatal("Cmd が発行されていない")
	}
	msg, ok := cmd().(hostreq.Msg)
	if !ok {
		t.Fatalf("Msg の型 = %T, want hostreq.Msg", cmd())
	}
	return msg
}

// 数えるのは対処が要る件数（WARN + FAIL）だけである。
//
// OK と SKIP を含めると、docker の無いホストで起動のたびに警告が出続ける。
func TestStartCountsOnlyActionableFindings(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		statuses []check.Status
		want     int
	}{
		"すべて正常":       {statuses: []check.Status{check.OK, check.OK}, want: 0},
		"SKIP は数えない":  {statuses: []check.Status{check.Skip, check.OK}, want: 0},
		"WARN を数える":   {statuses: []check.Status{check.Warn, check.OK}, want: 1},
		"FAIL を数える":   {statuses: []check.Status{check.Fail, check.OK}, want: 1},
		"WARN と FAIL": {statuses: []check.Status{check.Warn, check.Fail, check.Skip}, want: 2},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			checks := make([]doctor.Check, 0, len(tt.statuses))
			for i, st := range tt.statuses {
				checks = append(checks, stub{id: string(rune('a' + i)), status: st, runs: nil})
			}
			if got := run(t, hostreq.Start(doctor.Input{}, checks)).Bad; got != tt.want {
				t.Errorf("Bad = %d, want %d", got, tt.want)
			}
		})
	}
}

// 項目が無ければ Cmd を発行しない。
//
// nil を返すことで、呼び出し側は共有状態の配布だけの Cmd の形を変えずに済む。
func TestStartWithoutChecks(t *testing.T) {
	t.Parallel()

	if cmd := hostreq.Start(doctor.Input{}, nil); cmd != nil {
		t.Error("項目が無いのに Cmd が発行された")
	}
}

// 各項目はちょうど 1 回だけ実行される。
func TestStartRunsEachCheckOnce(t *testing.T) {
	t.Parallel()

	var runs atomic.Int32
	checks := []doctor.Check{
		stub{id: "a", status: check.OK, runs: &runs},
		stub{id: "b", status: check.Fail, runs: &runs},
	}
	run(t, hostreq.Start(doctor.Input{}, checks))

	if got := runs.Load(); got != 2 {
		t.Errorf("実行回数 = %d, want 2", got)
	}
}

// 起動時に走らせてよいのは Startup が真の項目だけである（FR-44）。
//
// 到達性やディスク集計を含めると、起動が回線とディスクの状態に引きずられる。
func TestRegistryStartupSetIsSafeForStartup(t *testing.T) {
	t.Parallel()

	checks := doctor.Startup(doctor.Default())
	if len(checks) == 0 {
		t.Fatal("起動時の項目が 1 つも無い")
	}
	for _, c := range checks {
		if !c.Startup() {
			t.Errorf("%s は起動時の対象ではない", c.ID())
		}
		if got := c.Category(); got != check.CatJobReq {
			t.Errorf("%s の分類 = %q, want %q（ジョブ実行の前提に限る）", c.ID(), got, check.CatJobReq)
		}
	}
}
