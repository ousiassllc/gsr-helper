package cmdtest_test

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/ui/page"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/pagetest/cmdtest"
)

// Pump と Drained（run.go の「Model を進める」側）の検証を集める。
//
// **ガード自身を検証する。** Pump の maxSteps と宛先タブの検査はどちらも「静かに
// 緑になる」ことを防ぐために置かれていて、その doc がそう明言している。ガードが
// 効いているかを誰も見ていないと、**ガードを外しても全テストが緑のまま**になる
// （Issue #157）。
//
// Pump の型引数は `any` なので、tea.Model のフィクスチャは要らない。ここでは進み方が
// 一目で読める int を Model の代わりに使う。

// pumpTimeout は Pump が 1 本の Cmd を待つ上限。
//
// ここへ渡す Cmd はすべて即座に戻るので、締め切りが効くのは戻らない Cmd を
// 意図的に混ぜたときだけである。**速さの検証ではないので負荷の側へ倒す**
// （run.go の CmdTimeout と同じ考え方）。
const pumpTimeout = 5 * time.Second

// countStep は Msg を 1 つ受けるたびに 1 進める step。新しい Cmd は返さない。
func countStep(n int, _ tea.Msg) (int, tea.Cmd) { return n + 1, nil }

// never は決して満たされない cond。
func never(int) bool { return false }

// msgCmd は marker を 1 つ返す Cmd。
func msgCmd() tea.Cmd { return func() tea.Msg { return marker{} } }

// 辿れる Cmd が尽きても条件を満たさなければ、その旨を返す。
//
// **黙って現在の Model を返してはならない。** 返すと呼び出し側は「進めた結果
// こうなった」と読み、実際には配線が足りずに 1 手も進まなかった場合と区別が付かない。
func TestPumpReportsExhaustedBundle(t *testing.T) {
	t.Parallel()

	got, err := cmdtest.Pump(0, 0, msgCmd(), pumpTimeout, countStep, never)
	if err == nil {
		t.Fatal("辿り尽くしたのに err が nil である")
	}
	if !strings.Contains(err.Error(), "尽きた") {
		t.Errorf("err = %v, 辿り尽くしたことを伝えていない", err)
	}
	if got != 1 {
		t.Errorf("進めた手数 = %d, want 1（尽きるまでに配れた Msg の数）", got)
	}
}

// 宛先の違う page.TabMsg は取り違えずに落とす。
//
// **これは静かに緑になることを防ぐガードである。** page.Do は結果を発行元のタブへ
// 戻す包みを付ける（page.TabMsg の doc）。宛先を見ずに中身を配ると、別のタブ宛の
// 結果でこのタブを進めてしまい、配送の誤りが検証を通り抜ける。
func TestPumpRejectsMsgForAnotherTab(t *testing.T) {
	t.Parallel()

	cmd := page.Do(3, func() tea.Msg { return marker{} })
	_, err := cmdtest.Pump(0, 0, cmd, pumpTimeout, countStep, never)
	if err == nil {
		t.Fatal("宛先の違う page.TabMsg で err が nil である")
	}
	if !strings.Contains(err.Error(), "宛先のタブ番号") {
		t.Errorf("err = %v, 宛先違いであることを伝えていない", err)
	}
}

// nil の Msg は読み飛ばし、後続の Msg で進む。
//
// tea.Cmd は nil を返しうる（何も起きなかったことを表す）。これを step へ渡すと
// タブ側が nil の型スイッチに落ちるため、配る前に捨てる。
func TestPumpSkipsNilMsg(t *testing.T) {
	t.Parallel()

	cmd := func() tea.Msg {
		return []tea.Cmd{func() tea.Msg { return nil }, msgCmd()}
	}
	// **step が何を受けたかで見る。** 手数だけを見ると、nil をそのまま配る実装でも
	// 同じ 1 手になって緑のまま通り抜ける（読み飛ばしの検査になっていない）。
	var seen []tea.Msg
	step := func(n int, msg tea.Msg) (int, tea.Cmd) {
		seen = append(seen, msg)

		return n + 1, nil
	}

	got, err := cmdtest.Pump(0, 0, cmd, pumpTimeout, step, func(n int) bool { return n == 1 })
	if err != nil {
		t.Fatalf("nil の Msg で止まった: %v", err)
	}
	if got != 1 {
		t.Errorf("進めた手数 = %d, want 1（nil は数えない）", got)
	}
	if len(seen) != 1 {
		t.Fatalf("step が受けた Msg = %d 件, want 1 件", len(seen))
	}
	if _, ok := seen[0].(marker); !ok {
		t.Errorf("step が受けたのは %#v, want marker（nil が配られている）", seen[0])
	}
}

// 条件を満たさない Cmd の連鎖は maxSteps で打ち切る。
//
// **打ち切らないとテストが終わらない。** step が新しい Cmd を返し続ける形（購読の
// 待ち受けなど）ではキューが減らないため、上限が無ければ無限に回る。
func TestPumpStopsAtMaxSteps(t *testing.T) {
	t.Parallel()

	// step が毎回 1 本ずつ新しい Cmd を返すので、キューは決して尽きない。
	step := func(n int, _ tea.Msg) (int, tea.Cmd) { return n + 1, msgCmd() }

	got, err := cmdtest.Pump(0, 0, msgCmd(), pumpTimeout, step, never)
	if err == nil {
		t.Fatal("尽きない連鎖なのに err が nil である（打ち切られていない）")
	}
	if !strings.Contains(err.Error(), "進めても条件を満たさなかった") {
		t.Errorf("err = %v, 打ち切りであることを伝えていない", err)
	}
	if got != 200 {
		t.Errorf("進めた手数 = %d, want 200（maxSteps）", got)
	}
}

// 条件を先に満たしていれば 1 手も進めずに返す。
func TestPumpReturnsWhenConditionAlreadyHolds(t *testing.T) {
	t.Parallel()

	got, err := cmdtest.Pump(7, 0, blocked(), pumpTimeout, countStep, func(n int) bool { return n == 7 })
	if err != nil {
		t.Fatalf("満たしているのに err が返った: %v", err)
	}
	if got != 7 {
		t.Errorf("Model = %d, want 7（進めない）", got)
	}
}

// Drained は閉じたチャネルで真を、閉じないチャネルで偽を返す。
//
// **偽の側が要点である。** 購読の後始末を確かめる検証はここが偽になったことで
// 「畳めていない」と判断するので、この腕が壊れると後始末の漏れが緑になる。
func TestDrainedTellsClosedApartFromOpen(t *testing.T) {
	t.Parallel()

	closed := make(chan int)
	close(closed)
	if !cmdtest.Drained(closed, pumpTimeout) {
		t.Error("閉じたチャネルで偽が返った")
	}

	// 値を 1 つ残したまま閉じても真を返す（残った値は読み捨てる）。
	left := make(chan int, 1)
	left <- 1
	close(left)
	if !cmdtest.Drained(left, pumpTimeout) {
		t.Error("値を残して閉じたチャネルで偽が返った")
	}

	// 閉じないチャネルは締め切りで諦める。ここは待つのが目的なので短く渡す。
	open := make(chan int)
	if cmdtest.Drained(open, 10*time.Millisecond) {
		t.Error("閉じていないチャネルで真が返った")
	}
}
