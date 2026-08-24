package discovery

import (
	"errors"
	"testing"
	"time"

	"github.com/ousiassllc/gsr-helper/internal/runner"
)

// 検出周期の進行状況（二重起動の抑止・次の Tick の予約・追い抜かれた周期の破棄）を
// 検証する。以前は親 Model（ui/refresh_test.go）が実行中の本数を直に覗いていた。

// input は 1 周期分の入力。検出そのものは Cmd の中でしか起きないので、ここでは
// Cmd を実行せず発行の有無だけを見る（Every だけは Tick の予約に効く）。
func input() Input {
	return Input{Every: time.Millisecond, Roots: nil, Depth: 1, Exec: nil}
}

// 実行中の検出があるうちは、Tick が来ても新しい検出を始めない。
//
// 自動更新間隔（下限 1 秒）は 1 回の検出に許す時間（Budget = 15 秒）より短いため、
// 無条件に発行すると周期が重なり、systemctl が応答しない状況で最大 40 プロセスが
// 同時に走る（State.Start）。
func TestOnTickDoesNotOverlapRuns(t *testing.T) {
	var s State

	if s.OnTick(input()) == nil {
		t.Fatal("1 回目の Tick で Cmd が発行されていない")
	}
	if !s.Busy() || s.Seq() != 1 {
		t.Fatalf("1 回目の Tick で検出が始まっていない（busy = %v, seq = %d）", s.Busy(), s.Seq())
	}

	// 結果が返る前に次の Tick が来ても検出は始めない。
	s.OnTick(input())
	if s.Seq() != 1 {
		t.Errorf("検出中の Tick で検出が重ねられた（seq = %d, want 1）", s.Seq())
	}

	// 結果が返れば次の周期から検出を再開する。
	s.Apply(Msg{Seq: 1, Result: runner.Result{}, Err: nil})
	if s.Busy() {
		t.Fatal("結果を受けても実行中のままになっている")
	}
	s.OnTick(input())
	if s.Seq() != 2 {
		t.Errorf("検出が再開されていない（seq = %d, want 2）", s.Seq())
	}
}

// 実行中は Start が nil を返す（始めなかったことを呼び出し側へ伝える形）。
//
// 手動の再読み込み（r）の連打もこの判定で塞ぐ。
func TestStartReturnsNilWhileBusy(t *testing.T) {
	var s State

	if s.Start(input()) == nil {
		t.Fatal("1 本目が始まらない")
	}
	if s.Start(input()) != nil {
		t.Error("実行中なのに 2 本目が始まっている（周期が重なり最大 40 プロセスが走る）")
	}
}

// 検出を始めなかった周期でも次の Tick は必ず予約する。落とすと自動更新がそこで
// 止まったままになる（State.OnTick）。
func TestOnTickAlwaysSchedulesNextTick(t *testing.T) {
	var s State
	s.OnTick(input())
	if !s.Busy() {
		t.Fatal("1 回目の Tick で検出が始まっていない（前提が崩れている）")
	}

	cmd := s.OnTick(input())
	if cmd == nil {
		t.Fatal("検出中の Tick で次の Tick が予約されていない（自動更新が止まる）")
	}
	// 予約されたのが Tick であることまで見る（検出の Cmd が返ると重ねたことになる）。
	if _, ok := cmd().(TickMsg); !ok {
		t.Errorf("検出中の Tick が返した Msg = %T, want TickMsg", cmd())
	}
}

// 追い抜かれた周期の結果は捨てる。番号が無いと遅い検出が後から返って一覧が
// 古い内容へ巻き戻る（Msg.Seq）。
func TestApplyDropsStaleCycle(t *testing.T) {
	var s State
	newer := runner.Result{Warnings: []error{errStale}}

	// 2 周期ぶんが発行された後、新しい方（seq 2）の結果を先に取り込む。
	if out := s.Apply(Msg{Seq: 2, Result: newer, Err: nil}); out.Stale {
		t.Fatal("新しい周期の結果を捨てている")
	}
	if len(s.Result().Warnings) != 1 {
		t.Fatalf("新しい結果が取り込まれていない（警告 %d 件）", len(s.Result().Warnings))
	}

	// 遅れて返った古い周期（seq 1）の結果では上書きしない。
	out := s.Apply(Msg{Seq: 1, Result: runner.Result{}, Err: nil})
	if !out.Stale {
		t.Error("古い周期を捨てていない（親が共有状態を配り直してしまう）")
	}
	if len(s.Result().Warnings) != 1 {
		t.Errorf("古い周期の結果で上書きされた（警告 %d 件, want 1）", len(s.Result().Warnings))
	}
}

// errStale は結果の取り違えを見分けるための印。
var errStale = errors.New("新しい周期の結果")
