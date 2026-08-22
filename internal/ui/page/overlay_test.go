package page

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// 重なりの規則そのもの（キーは最上位だけ・esc は 1 枚・同じ種類は重ねない）を、
// 種類に依らない stub で検証する。具体的なモーダルを混ぜた検証は登録する側
// （page/runnerdetail）が持つ。

// 検証に使うモーダルの種類。どちらも stubModal を登録する。
const (
	kindFirst  ModalKind = "first"
	kindSecond ModalKind = "second"
)

// newOverlay はモーダルを 1 枚も開いていない状態で返す。
func newOverlay() Overlay {
	o := NewOverlay(testKeys(), testStyles(), true)
	o.Register(kindFirst, newStub("1 枚目"))
	o.Register(kindSecond, newStub("2 枚目"))
	o.SetSize(80, 20)
	return o
}

// sendOverlay はキーを順に送り、最後の Cmd を返す。
func sendOverlay(o Overlay, keys ...string) (Overlay, tea.Cmd) {
	var cmd tea.Cmd
	for _, k := range keys {
		o, cmd = o.Update(press(k))
	}
	return o, cmd
}

// 何も開いていない間はキーを受けても状態が変わらず、描画も空になる。
func TestOverlayInactive(t *testing.T) {
	o := newOverlay()
	if o.Active() {
		t.Fatal("開いていないのに Active が真である")
	}

	o, cmd := sendOverlay(o, "j", "enter", "esc")
	if cmd != nil {
		t.Error("開いていないのに Cmd が発行されている")
	}
	if o.Active() {
		t.Error("キー入力でモーダルが開いている")
	}
	if got := o.View(); got != "" {
		t.Errorf("表示 = %q, want 空", got)
	}
	if o.Hints() != nil {
		t.Error("開いていないのにフッタのヒントがある")
	}
	if got := stubOf(t, o, kindFirst).keys; len(got) != 0 {
		t.Errorf("開いていないモーダルにキーが届いている（%v）", got)
	}
}

// キーは最上位の 1 枚にのみ届く。背後のモーダルにも page にも流さない。
func TestOverlayDeliversToTopOnly(t *testing.T) {
	o := newOverlay()
	o.Open(kindFirst, nil)
	o, _ = sendOverlay(o, "j", "x")
	if got := len(stubOf(t, o, kindFirst).keys); got != 2 {
		t.Fatalf("最上位が受け取ったキー = %d 件, want 2", got)
	}

	o.Open(kindSecond, nil)
	o, _ = sendOverlay(o, "j", "x")
	if got := len(stubOf(t, o, kindFirst).keys); got != 2 {
		t.Errorf("背後のモーダルが受け取ったキー = %d 件, want 2（キーが背後に流れている）", got)
	}
	if got := len(stubOf(t, o, kindSecond).keys); got != 2 {
		t.Errorf("最上位が受け取ったキー = %d 件, want 2", got)
	}
}

// esc は 1 枚だけ閉じる。
func TestOverlayCloseOneByOne(t *testing.T) {
	o := newOverlay()
	o.Open(kindFirst, nil)
	o.Open(kindSecond, nil)

	o, _ = sendOverlay(o, "esc")
	if !o.Active() {
		t.Fatal("esc で 2 枚とも閉じている")
	}
	if !strings.Contains(o.View(), "1 枚目") {
		t.Error("1 枚閉じた後に背後のモーダルが最上位になっていない")
	}

	o, _ = sendOverlay(o, "esc")
	if o.Active() {
		t.Error("2 回目の esc でモーダルが閉じていない")
	}
}

// 同じ種類のモーダルは重ねず、最上位へ動かす（閉じられないモーダルを作らない）。
func TestOverlayDoesNotStackSameKind(t *testing.T) {
	o := newOverlay()
	o.OpenHelp()
	o.OpenHelp()

	o, _ = sendOverlay(o, "esc")
	if o.Active() {
		t.Error("ヘルプを 2 回開くと esc 1 回で閉じない")
	}
}

// Active は ChromeMsg.Modal に載せる値であり、開閉と一致する。
func TestOverlayActiveMatchesStack(t *testing.T) {
	o := newOverlay()
	steps := []struct {
		do   func(o *Overlay)
		want bool
		name string
	}{
		{func(o *Overlay) { o.Open(kindFirst, nil) }, true, "1 枚開く"},
		{func(o *Overlay) { o.OpenHelp() }, true, "ヘルプを重ねる"},
		{func(o *Overlay) { o.Close() }, true, "1 枚閉じる"},
		{func(o *Overlay) { o.Close() }, false, "全部閉じる"},
		{func(o *Overlay) { o.Close() }, false, "空でも閉じられる"},
	}
	for _, s := range steps {
		s.do(&o)
		if got := o.Active(); got != s.want {
			t.Errorf("%s の後の Active = %v, want %v", s.name, got, s.want)
		}
	}
}

// フッタのヒントと見出しは最上位のモーダルのものになる。
func TestOverlayHintsAndTitleFollowTop(t *testing.T) {
	o := newOverlay()
	o.Open(kindFirst, nil)
	if got := o.Hints(); len(got) != 1 || got[0].Key != "y" {
		t.Errorf("フッタのヒント = %+v, want 登録したモーダルのもの", got)
	}

	// ヘルプは閉じるキーを必ず出し、収まらないときだけスクロールのキーを添える。
	o.OpenHelp()
	hints := o.Hints()
	if len(hints) == 0 || hints[len(hints)-1].Key != "esc" {
		t.Errorf("ヘルプのヒント = %+v, want 末尾に esc", hints)
	}
}

// 共有状態と大きさは、開いていないモーダルにも配る。
//
// 開いた瞬間に古い配色・古い検出結果・古い大きさで描かれることを防ぐためである。
func TestOverlaySetStateReachesEveryModal(t *testing.T) {
	o := newOverlay()
	o.SetState(StateMsg{Keys: testKeys(), Styles: testStyles(), Dark: true, BodyW: 100, BodyH: 30})

	for _, kind := range []ModalKind{kindFirst, kindSecond} {
		stub := stubOf(t, o, kind)
		if stub.states != 1 {
			t.Errorf("%q が受け取った共有状態 = %d 件, want 1", kind, stub.states)
		}
		if stub.size.W == 0 || stub.size.W >= 100 {
			t.Errorf("%q が受け取った幅 = %d, want 枠の分を引いた値", kind, stub.size.W)
		}
	}
}
