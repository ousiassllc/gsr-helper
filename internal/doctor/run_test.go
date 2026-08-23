package doctor_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/ousiassllc/gsr-helper/internal/doctor"
	"github.com/ousiassllc/gsr-helper/internal/doctor/check"
)

// stub は Run の振る舞いだけを確かめるための Check。
type stub struct {
	id      string
	cat     string
	startup bool
	results []doctor.CheckResult
	hook    func()
}

func (s stub) ID() string       { return s.id }
func (s stub) Category() string { return s.cat }
func (s stub) Startup() bool    { return s.startup }

func (s stub) Run(context.Context, doctor.Input) []doctor.CheckResult {
	if s.hook != nil {
		s.hook()
	}
	return s.results
}

var _ doctor.Check = stub{}

func res(id, cat, target string, st doctor.Status) doctor.CheckResult {
	return doctor.CheckResult{
		ID: id, Category: cat, Target: target, Status: st,
		Summary: "", Detail: "", Impact: "", Remedy: "", Startup: false,
	}
}

// 各チェックは並列に実行される（FR-32）。
//
// 直列だと全員が揃うのを待てず、この検査は期限切れで落ちる。所要時間の比較では
// なく「揃うこと」で判定するのは、負荷の高い CI でも安定させるためである。
func TestRunExecutesChecksInParallel(t *testing.T) {
	t.Parallel()

	const n = 4
	var wg sync.WaitGroup
	wg.Add(n)
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()

	checks := make([]doctor.Check, 0, n)
	for i := range n {
		checks = append(checks, stub{
			id:  string(rune('a' + i)),
			cat: check.CatAuthz,
			hook: func() {
				wg.Done()
				select {
				case <-done:
				case <-time.After(5 * time.Second):
				}
			},
			startup: false,
			results: nil,
		})
	}

	go func() { doctor.Run(context.Background(), doctor.Input{}, checks) }()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("全チェックが同時に走らなかった（直列に実行されている）")
	}
}

// 並べる順は 分類 → 識別子 → 対象。完了順に並べるとカーソルの位置が意味を失う。
func TestRunSortsResults(t *testing.T) {
	t.Parallel()

	checks := []doctor.Check{
		stub{id: "z", cat: check.CatConsistency, startup: false, hook: nil, results: []doctor.CheckResult{
			res("consistency.orphan", check.CatConsistency, "", doctor.Warn),
		}},
		stub{id: "a", cat: check.CatJobReq, startup: false, hook: nil, results: []doctor.CheckResult{
			res("job.sudo", check.CatJobReq, "build02", doctor.Warn),
			res("job.sudo", check.CatJobReq, "build01", doctor.Warn),
			res("job.docker", check.CatJobReq, "", doctor.Fail),
		}},
		stub{id: "m", cat: check.CatAuthz, startup: false, hook: nil, results: []doctor.CheckResult{
			res("authz.perm", check.CatAuthz, "", doctor.OK),
		}},
	}

	got := doctor.Run(context.Background(), doctor.Input{}, checks)

	type key struct{ id, target string }
	want := []key{
		{"authz.perm", ""},
		{"job.docker", ""},
		{"job.sudo", "build01"},
		{"job.sudo", "build02"},
		{"consistency.orphan", ""},
	}
	if len(got) != len(want) {
		t.Fatalf("件数 = %d, want %d（%+v）", len(got), len(want), got)
	}
	for i, w := range want {
		if got[i].ID != w.id || got[i].Target != w.target {
			t.Errorf("%d 番目 = %s/%s, want %s/%s", i, got[i].ID, got[i].Target, w.id, w.target)
		}
	}
}

// 起動時の自動実行（FR-44）はレジストリの絞り込みで済ませる。
func TestStartupFiltersRegistry(t *testing.T) {
	t.Parallel()

	checks := []doctor.Check{
		stub{id: "job.docker", cat: check.CatJobReq, startup: true, hook: nil, results: nil},
		stub{id: "net.reach", cat: check.CatNetwork, startup: false, hook: nil, results: nil},
		stub{id: "job.sudo", cat: check.CatJobReq, startup: true, hook: nil, results: nil},
	}

	got := doctor.Startup(checks)
	if len(got) != 2 {
		t.Fatalf("件数 = %d, want 2", len(got))
	}
	for _, c := range got {
		if !c.Startup() {
			t.Errorf("%s は起動時の対象ではない", c.ID())
		}
	}
}

// 個別の再実行（FR-34）は識別子で引く。
func TestByID(t *testing.T) {
	t.Parallel()

	checks := []doctor.Check{
		stub{id: "job.docker", cat: check.CatJobReq, startup: true, hook: nil, results: nil},
		stub{id: "job.sudo", cat: check.CatJobReq, startup: true, hook: nil, results: nil},
	}

	got := doctor.ByID(checks, "job.sudo")
	if len(got) != 1 || got[0].ID() != "job.sudo" {
		t.Fatalf("ByID = %+v, want job.sudo 1 件", got)
	}
	if n := len(doctor.ByID(checks, "無い")); n != 0 {
		t.Errorf("未知の識別子で %d 件返った, want 0", n)
	}
}

func TestCount(t *testing.T) {
	t.Parallel()

	got := doctor.Count([]doctor.CheckResult{
		res("a", check.CatAuthz, "", doctor.OK),
		res("b", check.CatAuthz, "", doctor.OK),
		res("c", check.CatAuthz, "", doctor.Warn),
		res("d", check.CatAuthz, "", doctor.Fail),
		res("e", check.CatAuthz, "", doctor.Skip),
	})
	want := doctor.Summary{OK: 2, Warn: 1, Fail: 1, Skip: 1}
	if got != want {
		t.Fatalf("Count = %+v, want %+v", got, want)
	}
	if got.Bad() != 2 {
		t.Errorf("Bad() = %d, want 2（WARN + FAIL）", got.Bad())
	}
	if got.Total() != 5 {
		t.Errorf("Total() = %d, want 5", got.Total())
	}
}

// 結果が 1 件も無いときも空のスライスで返る（nil でも len 0 でもよいが panic しない）。
func TestRunWithNoChecks(t *testing.T) {
	t.Parallel()

	if got := doctor.Run(context.Background(), doctor.Input{}, nil); len(got) != 0 {
		t.Errorf("件数 = %d, want 0", len(got))
	}
}
