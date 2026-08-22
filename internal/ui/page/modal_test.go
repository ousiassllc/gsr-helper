package page

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

// モーダルと page のあいだの戻り道（Issue #26）を、種類に依らない stub で検証する。
// 決定が page へ戻ること・結果が発行元のタブへ戻ること・閉じた後の結果が宛先を
// 失わないこと・esc を最上位が先に解釈できること・登録と開封の Cmd が伝わること。

// 登録したモーダルは自分が乗っているタブ番号を受け取る。
//
// 番号を知らないと決定もドメイン呼び出しも Do で包めず、結果が「そのとき選択中の
// タブ」へ配られて静かに失われる（page.AttachMsg の doc）。
func TestOverlayAttachesTabNumber(t *testing.T) {
	o := newOverlay()
	for _, kind := range []ModalKind{kindFirst, kindSecond} {
		if got := stubOf(t, o, kind).tab; got != testTab {
			t.Errorf("%q が受け取ったタブ番号 = %d, want %d", kind, got, testTab)
		}
	}
}

// 決定に載ったタブ番号は、タブを切り替えても発行元へ戻す包みになる。
//
// Do で包んだ Cmd の結果は TabMsg として親へ上がり、親が発行元のタブへ差し戻す
// （page.TabMsg の doc）。
func TestModalResultCarriesOriginTab(t *testing.T) {
	const other ModalKind = "other"

	cmd := Do(testTab, func() tea.Msg { return ResultMsg{Kind: other, Msg: "決定"} })
	got, ok := cmd().(TabMsg)
	if !ok {
		t.Fatalf("包んだ結果 = %T, want TabMsg", cmd())
	}
	if got.Tab != testTab {
		t.Errorf("差し戻し先のタブ = %d, want %d", got.Tab, testTab)
	}
	res, ok := got.Msg.(ResultMsg)
	if !ok || res.Kind != other {
		t.Errorf("包まれた決定 = %+v, want %q の ResultMsg", got.Msg, other)
	}
}

// 決定（ResultMsg）は Overlay が受け取らない。page が自分の switch で解釈する。
//
// Overlay へ戻すと決定は発行元のモーダル自身へ帰り、そこで捨てられる。
func TestOverlayDoesNotHandleResult(t *testing.T) {
	o := newOverlay()
	o.Open(kindFirst, nil)
	if !o.Active() {
		t.Fatal("モーダルが開いていない（前提が崩れている）")
	}

	if o.Handles(ResultMsg{Kind: kindFirst, Msg: "決定"}) {
		t.Error("モーダルを開いている間に決定が Overlay へ渡っている")
	}
	if !o.Handles(tea.WindowSizeMsg{Width: 1, Height: 1}) {
		t.Error("開いている間の通常の Msg が Overlay へ渡らない")
	}
	if o.Handles(tea.WindowSizeMsg{Width: 1, Height: 1}) != o.Active() {
		t.Error("通常の Msg の配送先が開閉と一致しない")
	}
}

// 宛先を明示した Msg は、閉じた後でも最上位でなくてもその種類へ届く。
//
// 最上位にしか渡さないと、背後のモーダル宛の結果は最上位に食われ、モーダルを
// 閉じた後に届いた結果は誰にも届かない（page.ModalMsg の doc）。
func TestOverlayAddressedMsgReachesClosedModal(t *testing.T) {
	o := newOverlay()
	if !o.Handles(ModalMsg{Kind: kindFirst, Msg: "結果"}) {
		t.Fatal("閉じている間に宛先付きの Msg が捨てられている")
	}

	// 1 枚も開いていない状態で届く。
	o, _ = o.Update(ModalMsg{Kind: kindFirst, Msg: "閉じた後の結果"})
	if got := stubOf(t, o, kindFirst).msgs; len(got) != 1 || got[0] != "閉じた後の結果" {
		t.Errorf("閉じた後に届いた Msg = %v, want 1 件", got)
	}

	// 別のモーダルが最上位でも、宛先のモーダルへ届く。
	o.Open(kindSecond, nil)
	o, _ = o.Update(ModalMsg{Kind: kindFirst, Msg: "背後への結果"})
	if got := len(stubOf(t, o, kindFirst).msgs); got != 2 {
		t.Errorf("背後のモーダルが受け取った Msg = %d 件, want 2（最上位に食われている）", got)
	}
	if got := stubOf(t, o, kindSecond).msgs; len(got) != 0 {
		t.Errorf("最上位が宛先外の Msg を受け取っている（%v）", got)
	}
}

// esc は最上位のモーダルが先に解釈でき、消費しなかったときだけ 1 枚閉じる。
func TestOverlayBackReachesTopModalFirst(t *testing.T) {
	o := newOverlay()
	o.Open(kindFirst, nil)
	stubOf(t, o, kindFirst).back = true

	o, _ = sendOverlay(o, "esc")
	if !o.Active() {
		t.Fatal("最上位が esc を解釈するのにモーダルが閉じている")
	}
	if got := stubOf(t, o, kindFirst).keys; len(got) != 1 || got[0] != "esc" {
		t.Errorf("最上位が受け取ったキー = %v, want [esc]", got)
	}

	// 解釈しなくなったら、次の esc で閉じる。
	stubOf(t, o, kindFirst).back = false
	o, _ = sendOverlay(o, "esc")
	if o.Active() {
		t.Error("esc を解釈しないモーダルが閉じていない")
	}
	if got := len(stubOf(t, o, kindFirst).keys); got != 1 {
		t.Errorf("閉じる esc がモーダルへも渡っている（受け取ったキー = %d 件）", got)
	}
}

// 登録と開封が返す Cmd は呼び出し側まで伝わる。
//
// 捨てると、開いた瞬間に購読や計算を始めるモーダルがその処理を動かせない。
func TestOverlayRegisterAndOpenReturnCmd(t *testing.T) {
	const kindThird ModalKind = "third"

	o := newOverlay()
	if cmd := o.Register(kindThird, newStub("3 枚目")); cmd == nil {
		t.Error("Register が Cmd を返していない")
	}
	if cmd := o.Open(kindThird, "開く指示"); cmd == nil {
		t.Error("Open が Cmd を返していない")
	}
	if cmd := o.Open("未登録", "開く指示"); cmd != nil {
		t.Error("未登録の種類で Cmd が返っている")
	}
}
