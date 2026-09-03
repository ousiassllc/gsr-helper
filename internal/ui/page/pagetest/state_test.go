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
	if got := pagetest.State(80, 16); got.Deps.Exec == nil {
		t.Error("共有状態のフィクスチャが Exec を持たない")
	}
	if got := pagetest.State(80, 16, pagetest.SampleRunner()); got.Deps.Exec == nil {
		t.Error("runner を渡した共有状態のフィクスチャが Exec を持たない")
	}
}

// 共有状態のフィクスチャはプロセス走査を必ず持つ。
//
// Exec と同じ理由でここに置く。nil のまま配ると svc 側は procs.Scan に落ち
// （page.Deps.ScanProcs の doc）、ドレイン停止（FR-07）の停止条件が**テストを
// 走らせるホストの /proc** で決まる。落としても現状のテストは緑のままなので、
// 外れたことに気付けるのはこの 1 件だけである（Issue #155）。
func TestStateAlwaysCarriesProcScan(t *testing.T) {
	if got := pagetest.State(80, 16); got.Deps.ScanProcs == nil {
		t.Error("共有状態のフィクスチャがプロセス走査を持たない")
	}
	if got := pagetest.State(80, 16, pagetest.SampleRunner()); got.Deps.ScanProcs == nil {
		t.Error("runner を渡した共有状態のフィクスチャがプロセス走査を持たない")
	}
}

// プロセス走査は渡した runner の Workers をそのまま返す。
//
// **上の nil 検査だけでは足りない。** 走査を
// `func() ([]runner.Process, error) { return nil, nil }` に潰しても nil 検査は緑の
// ままで、そのとき ScanOf の doc が約束する「一覧が busy と出している runner は走査
// でも busy」が黙って失われる。busy な対象のドレイン停止を辿るテストは以後、待機
// せずに即座に停止条件を満たして**空虚に緑**になり、Issue #155 が消したはずの欠陥が
// そのまま戻る。閉包を実際に呼ぶこの 1 件がその潰しを落とす。
func TestStateProcScanReportsRunnerWorkers(t *testing.T) {
	got, err := pagetest.State(80, 16, pagetest.BusyRunner()).Deps.ScanProcs()
	if err != nil {
		t.Fatalf("busy な runner の走査が失敗した: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("走査が返したプロセス = %d 件, want 1 件（BusyRunner の Workers）", len(got))
	}
	want := pagetest.BusyRunner().Workers[0]
	if got[0].PID != want.PID || got[0].Kind != want.Kind || got[0].Dir != want.Dir {
		t.Errorf("走査が返したプロセス = PID %d/%v/%s, want PID %d/%v/%s",
			got[0].PID, got[0].Kind, got[0].Dir, want.PID, want.Kind, want.Dir)
	}

	// Workers を持たない runner では空になる（走査がホストの /proc を見ていない）。
	idle, err := pagetest.State(80, 16, pagetest.SampleRunner()).Deps.ScanProcs()
	if err != nil {
		t.Fatalf("稼働中の runner の走査が失敗した: %v", err)
	}
	if len(idle) != 0 {
		t.Errorf("走査が返したプロセス = %d 件, want 0 件（SampleRunner は Workers を持たない）", len(idle))
	}
}
