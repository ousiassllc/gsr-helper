package ui

import (
	"errors"
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/exec"
	"github.com/ousiassllc/gsr-helper/internal/runner"
	"github.com/ousiassllc/gsr-helper/internal/ui/discovery"
)

// 実行中の検出があるうちは、Tick が来ても新しい検出を始めない。
//
// 自動更新間隔（下限 1 秒）は 1 回の検出に許す時間（15 秒）より短いため、無条件に
// 発行すると周期が重なり、systemctl が応答しない状況で最大 40 プロセスが同時に走る
// （discover.go の onTick）。
func TestTickSkipsDiscoverWhileOneIsRunning(t *testing.T) {
	a := newApp(exec.NewFake())

	// Tick の Cmd は実行しない（自動更新間隔だけ待つ）。検出を始めたかは
	// 実行中の本数で見る。
	a, cmd := update(a, tickMsg{})
	if a.inflight != 1 || cmd == nil {
		t.Fatalf("1 回目の tickMsg で検出が始まっていない（inflight = %d）", a.inflight)
	}

	// 結果が返る前に次の Tick が来ても検出は始めない（次の Tick だけを予約する）。
	a, cmd = update(a, tickMsg{})
	if a.inflight != 1 {
		t.Errorf("検出中の tickMsg で検出が重ねられた（inflight = %d, want 1）", a.inflight)
	}
	if cmd == nil {
		t.Error("次の Tick が予約されていない（自動更新が止まる）")
	}

	// 結果が返れば次の周期から検出を再開する。
	a, _ = update(a, discovery.Msg{Seq: 1, Result: runner.Result{}, Err: nil})
	if a.inflight != 0 {
		t.Fatalf("結果を受けた後の実行中の本数 = %d, want 0", a.inflight)
	}
	if a, _ = update(a, tickMsg{}); a.inflight != 1 {
		t.Error("検出が再開されていない")
	}
}

// 手動の再読み込み（r）も実行中の検出には重ねない。
func TestRefreshKeySkipsDiscoverWhileOneIsRunning(t *testing.T) {
	a, _ := update(newApp(exec.NewFake()), tickMsg{})

	// 打鍵は page を経由して差し戻される（keys.go の handleKey）。差し戻しを
	// 処理した結果に検出が含まれないことを見る。
	if next, _ := sendKey(a, "r"); next.inflight != 1 {
		t.Errorf("検出中の r で検出が重ねられた（inflight = %d, want 1）", next.inflight)
	}
}

// 追い抜かれた周期の結果は捨てる。番号が無いと遅い検出が後から返って一覧が
// 古い内容へ巻き戻る（discovery.Msg.Seq）。
func TestStaleDiscoverResultDoesNotOverwrite(t *testing.T) {
	a := newApp(exec.NewFake())
	newer := runner.Result{Warnings: []error{errStale}}

	// 2 周期ぶんを発行し、新しい方（seq 2）の結果を先に取り込む。
	a, _ = update(a, discovery.Msg{Seq: 2, Result: newer, Err: nil})
	if len(a.result.Warnings) != 1 {
		t.Fatalf("新しい結果が取り込まれていない（警告 %d 件）", len(a.result.Warnings))
	}

	// 遅れて返った古い周期（seq 1）の結果では上書きしない。
	a, cmd := update(a, discovery.Msg{Seq: 1, Result: runner.Result{}, Err: nil})
	if len(a.result.Warnings) != 1 {
		t.Errorf("古い周期の結果で上書きされた（警告 %d 件, want 1）", len(a.result.Warnings))
	}
	if cmd != nil {
		t.Error("古い周期の結果で共有状態を配り直している")
	}
}

// errStale は結果の取り違えを見分けるための印。
var errStale = errors.New("新しい周期の結果")
