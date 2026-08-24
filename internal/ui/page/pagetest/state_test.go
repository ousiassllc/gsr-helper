package pagetest_test

import (
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/ui/page/pagetest"
)

// 共有状態のフィクスチャは Exec を必ず持つ。
//
// page.StateMsg は「systemctl が無い環境でも Exec を nil にはしない」と定めており
// （page.go の Exec の doc）、親は実際に nil を配らない。フィクスチャがこれを破ると、
// タブは**親が決して作らない状態**でしか検証されないことになる。jobs と runners が
// 私物の testState を持っていた頃がその状態だった（Issue #31）。
//
// この検査をフィクスチャ側に 1 つ置くことで、State を使うすべてのタブが不変条件を
// 引き継ぐ。
func TestStateAlwaysCarriesExecutor(t *testing.T) {
	if got := pagetest.State(80, 16); got.Exec == nil {
		t.Error("共有状態のフィクスチャが Exec を持たない")
	}
	if got := pagetest.State(80, 16, pagetest.SampleRunner()); got.Exec == nil {
		t.Error("runner を渡した共有状態のフィクスチャが Exec を持たない")
	}
}

// 共有状態のフィクスチャはプロセス走査を必ず持つ。
//
// Exec と同じ理由でここに置く。nil のまま配ると svc 側は procs.Scan に落ち
// （page.StateMsg.ScanProcs の doc）、ドレイン停止（FR-07）の停止条件が**テストを
// 走らせるホストの /proc** で決まる。落としても現状のテストは緑のままなので、
// 外れたことに気付けるのはこの 1 件だけである（Issue #155）。
func TestStateAlwaysCarriesProcScan(t *testing.T) {
	if got := pagetest.State(80, 16); got.ScanProcs == nil {
		t.Error("共有状態のフィクスチャがプロセス走査を持たない")
	}
	if got := pagetest.State(80, 16, pagetest.SampleRunner()); got.ScanProcs == nil {
		t.Error("runner を渡した共有状態のフィクスチャがプロセス走査を持たない")
	}
}
