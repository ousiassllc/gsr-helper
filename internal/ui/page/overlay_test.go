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
	o, _ := NewOverlay(testTab, state(80, 20))
	o.Register(kindFirst, newStub("1 枚目"))
	o.Register(kindSecond, newStub("2 枚目"))
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

// 共有状態は開いているモーダルにだけ配り、開く直前に最新をリプレイする。
//
// 閉じているモーダルにも毎周期配ると、誰も見ていないヘルプを 3 秒ごとに全行
// 組み直すような無駄が積み上がる（Issue #32）。古い状態で描かれないことは
// Open のリプレイが担保する。
func TestOverlaySetStateReachesOpenModalsOnly(t *testing.T) {
	o := newOverlay()
	base := stubOf(t, o, kindFirst).states

	o.SetState(state(100, 30))
	if got := stubOf(t, o, kindFirst).states; got != base {
		t.Errorf("閉じているモーダルへ共有状態が配られている（%d 件）", got-base)
	}

	// 開く直前にリプレイされるので、開いた時点で最新を持っている。
	o.Open(kindFirst, nil)
	first := stubOf(t, o, kindFirst)
	if first.states != base+1 {
		t.Errorf("開いた時点の共有状態 = %d 件, want %d 件", first.states, base+1)
	}
	if first.size.W == 0 || first.size.W >= 100 {
		t.Errorf("開いた時点の幅 = %d, want 枠の分を引いた値", first.size.W)
	}

	// 以後の周期は開いているものだけが受け取る。
	o.SetState(state(100, 30))
	if got := stubOf(t, o, kindFirst).states; got != base+2 {
		t.Errorf("開いているモーダルの共有状態 = %d 件, want %d 件", got, base+2)
	}
	if got := stubOf(t, o, kindSecond).states; got != base {
		t.Errorf("閉じているモーダルへ共有状態が配られている（%d 件）", got-base)
	}
}

// 起動後に遅延登録したモーダルは、登録した時点で最新の共有状態と領域を受け取る。
//
// 配られた中身は wantFullState が見る（件数だけを数えると Issue #32 の症状を
// 見逃す理由も、そこに書いてある）。
func TestOverlayReplaysStateOnRegister(t *testing.T) {
	const kindLate ModalKind = "late"

	o := newOverlay()
	o.SetState(state(120, 40))
	o.Register(kindLate, newStub("遅延登録"))

	late := stubOf(t, o, kindLate)
	if late.states == 0 {
		t.Fatal("遅延登録したモーダルへ共有状態が配られていない")
	}
	wantFullState(t, late, 120, 40)
}

// 寿命の通知は page 本体のものであり、モーダルが開いていても Overlay へ渡さない。
//
// 渡すと、モーダルを開いたままタブを切り替えた／終了したときに page が長寿命の
// 処理を畳む機会を失い、通知はモーダル（最終的に viewport）に飲まれて消える
// （Issue #41 の契約）。
func TestOverlayDoesNotHandleLifecycleMsgs(t *testing.T) {
	o := newOverlay()
	o.Open(kindFirst, nil)
	if !o.Active() {
		t.Fatal("モーダルが開いていない（前提が崩れている）")
	}

	lifecycle := map[string]tea.Msg{
		"裏へ回る":  DeactivateMsg{},
		"前面へ戻る": ActivateMsg{},
		"終了する":  ShutdownMsg{},
	}
	for name, msg := range lifecycle {
		t.Run(name, func(t *testing.T) {
			if o.Handles(msg) {
				t.Errorf("%T が Overlay へ配られている（page 本体に届かない）", msg)
			}
		})
	}

	// 既定（開いていれば渡す）は生きている。上の 3 つだけを外していることを見る。
	if !o.Handles(struct{}{}) {
		t.Error("開いているのに既定の Msg が Overlay へ配られない")
	}
}

// SizeMsg は領域が変わったときだけ配る。
//
// 変わっていない領域を配ると、下位に再計算を強いるだけである（Issue #32）。
func TestOverlaySendsSizeOnlyWhenChanged(t *testing.T) {
	o := newOverlay()
	o.Open(kindFirst, nil)
	before := stubOf(t, o, kindFirst).sizes

	o.SetState(state(100, 30))
	afterFirst := stubOf(t, o, kindFirst).sizes
	if afterFirst != before+1 {
		t.Fatalf("領域が変わった周期の SizeMsg = %d 回, want 1 回", afterFirst-before)
	}

	o.SetState(state(100, 30))
	if got := stubOf(t, o, kindFirst).sizes; got != afterFirst {
		t.Errorf("領域が変わらない周期に SizeMsg が %d 回配られている", got-afterFirst)
	}

	o.SetState(state(90, 30))
	if got := stubOf(t, o, kindFirst).sizes; got != afterFirst+1 {
		t.Errorf("領域が変わった周期の SizeMsg = %d 回, want 1 回", got-afterFirst)
	}
}

// 写しは重なりの実体を共有する。開閉も中身も 1 つの状態に集まる。
//
// 以前はスタックだけがスライスの付け替えで写しごとに分かれ、map だけが共有される
// 半端な状態だった。捨てた写しが中身の変更だけを残し、開閉の変更を失う
// （Issue #32）。
func TestOverlayCopySharesState(t *testing.T) {
	o := newOverlay()
	clone := o

	clone.Open(kindFirst, nil)
	if !o.Active() {
		t.Error("写しで開いたモーダルが元の値に見えない（開閉が分かれている）")
	}

	o.Close()
	if clone.Active() {
		t.Error("元の値で閉じたモーダルが写しに残っている")
	}
}
