package dialog_test

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"charm.land/bubbles/v2/stopwatch"
	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/ui/keymap"
	"github.com/ousiassllc/gsr-helper/internal/ui/organism/dialog"
)

// 待機画面の計時が「実際に進む」ことを固定する。
//
// **Start が返す Cmd を流さないと計時は動かない。** stopwatch は開始の指示
// （StartStopMsg）を受けるまで Tick を捨てるため、Cmd を捨てて Tick だけ配るテストは
// 「経過 0s のまま」を正常として緑になる（PR #70 CRITICAL C1）。ここでは Cmd を
// ランタイムと同じ規則で辿って配り、経過が増えることまで見る。

// startedWaiter は待機画面を組み立て、Start が返す Cmd を配って動かし始める。
// 併せて計時の ID を返す（Tick は ID が一致しないと捨てられる）。
func startedWaiter(t *testing.T, in dialog.DrainInput) (dialog.DrainWaiter, int) {
	t.Helper()

	d := dialog.NewDrainWaiter(keymap.NewGlobal(), testStyles())
	d.SetSize(drainWidth, drainHeight)
	d.SetInput(in)

	return d, startWaiter(t, &d)
}

// startWaiter は Start が返す Cmd を配り直し、計時の ID を返す。
//
// **計時の Tick だけは配らない。** 何秒経ったことにするかはテストが tickWaiter で
// 決める（cmdtest.Pump が Tick を配らないのと同じ理由）。stopwatch は自分の Tick を
// Update で繋いで回るため、配ると刻みが二重になって数え方が読めなくなる。
func startWaiter(t *testing.T, d *dialog.DrainWaiter) int {
	t.Helper()

	id := 0
	for _, msg := range cmdMsgs(d.Start()) {
		if sw, ok := msg.(stopwatch.StartStopMsg); ok {
			id = sw.ID
		}
		if _, ok := msg.(stopwatch.TickMsg); ok {
			continue
		}
		*d, _ = d.Update(msg)
	}
	if id == 0 {
		t.Fatal("Start() の Cmd に stopwatch.StartStopMsg が入っていない（計時が始まらない）")
	}
	return id
}

// tickWaiter は計時の Tick を 1 回配る。
func tickWaiter(d *dialog.DrainWaiter, id int) {
	*d, _ = d.Update(stopwatch.TickMsg{ID: id})
}

// cmdMsgs は Cmd を実行し、束（tea.Batch / tea.Sequence の結果）なら中身も辿って
// 出てきた Msg を平坦に返す。
//
// bubbletea のランタイムが束を再帰的に取り出すのと同じ規則である
// （Program.execBatchMsg / execSequenceMsg）。**reflect で束を見分けるのは
// tea.Sequence の実体が非公開型で型アサーションでは捕まえられないためである。**
// 束はどちらも []tea.Cmd を素の型に持つので、要素型で判別する。
func cmdMsgs(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}

	msg := cmd()
	v := reflect.ValueOf(msg)
	if v.Kind() != reflect.Slice || v.Type().Elem() != reflect.TypeFor[tea.Cmd]() {
		return []tea.Msg{msg}
	}

	out := make([]tea.Msg, 0, v.Len())
	for i := range v.Len() {
		c, ok := v.Index(i).Interface().(tea.Cmd)
		if !ok {
			continue
		}
		out = append(out, cmdMsgs(c)...)
	}
	return out
}

// 経過時間を出す。計時は bubbles/stopwatch に委ね、表記は atom.Duration に揃える。
//
// 進捗バーは出さない。待ち時間は無制限（FR-07）で完了時期を約束できないためである
// （atomic-design.md の「bubbles/progress を使う範囲」）。
func TestDrainWaiterShowsElapsed(t *testing.T) {
	d, id := startedWaiter(t, oneJob())

	if got := d.Elapsed(); got != 0 {
		t.Errorf("Elapsed() = %v, want 0（開始直後）", got)
	}
	if want := "経過 0s"; !strings.Contains(d.View(), want) {
		t.Errorf("%q が描かれていない:\n%s", want, d.View())
	}

	tickWaiter(&d, id)

	if got := d.Elapsed(); got != time.Second {
		t.Errorf("Tick を 1 回配った後の Elapsed() = %v, want 1s（計時が進んでいない）", got)
	}
	if want := "経過 1s"; !strings.Contains(d.View(), want) {
		t.Errorf("%q が描かれていない:\n%s", want, d.View())
	}
	if strings.Contains(d.View(), "%") {
		t.Errorf("進捗の割合が描かれている:\n%s", d.View())
	}
}

// 2 回目のドレインは経過 0 から始まる。
//
// 待機画面は Overlay へ 1 度だけ登録されて使い回される（runnerop.New）。Start が
// 計時を戻さないと、2 件目以降の対象でも 2 回目のドレインでも前回の経過が
// 引き継がれ、押した直後に「経過 5m」と出る。
func TestDrainWaiterStartResetsElapsed(t *testing.T) {
	d, id := startedWaiter(t, oneJob())
	tickWaiter(&d, id)
	tickWaiter(&d, id)

	if got := d.Elapsed(); got != 2*time.Second {
		t.Fatalf("1 回目の Elapsed() = %v, want 2s（前提が崩れている）", got)
	}

	// 1 回目の待機を終えて、同じ実体で 2 回目を開き直す。
	for _, msg := range cmdMsgs(d.Stop()) {
		d, _ = d.Update(msg)
	}
	startWaiter(t, &d)

	if got := d.Elapsed(); got != 0 {
		t.Errorf("2 回目の Start 直後の Elapsed() = %v, want 0（前回の経過を引き継いでいる）", got)
	}
	if want := "経過 0s"; !strings.Contains(d.View(), want) {
		t.Errorf("%q が描かれていない:\n%s", want, d.View())
	}
}

// 止めた後は計時が進まない。Stop の Cmd を流せないと待機を終えても数字が増え続ける。
func TestDrainWaiterStopHaltsElapsed(t *testing.T) {
	d, id := startedWaiter(t, oneJob())
	tickWaiter(&d, id)

	for _, msg := range cmdMsgs(d.Stop()) {
		d, _ = d.Update(msg)
	}
	tickWaiter(&d, id)

	if got := d.Elapsed(); got != time.Second {
		t.Errorf("停止後の Elapsed() = %v, want 1s（止めても計時が進んでいる）", got)
	}
}
