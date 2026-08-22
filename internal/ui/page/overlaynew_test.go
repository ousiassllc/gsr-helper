package page

import "testing"

// 組み立てた直後の Overlay が持つ共有状態を検証する。
//
// Register は登録した時点で最新の共有状態をリプレイする（Register の doc）ので、
// NewOverlay が持つ初期状態が欠けていると、その欠けがそのままモーダルへ配られる。

// 構築時に登録したモーダルは、最初の SetState を待たずに完全な共有状態を受け取る。
//
// **中身まで見る。** NewOverlay が初期の状態を StateMsg{Keys, Styles, Dark} だけで
// 組んでいた頃は、ここでリプレイされるのが Exec = nil・Caps ゼロ値・Result 空という
// 半端な状態だった。登録した時点で処理を始めるモーダルはその nil の Executor を
// 掴む（Issue #32 の受入条件 6 で runnerdetail の Caps ゼロ値センチネルを外したため、
// 掴んだことに気付く手立ても無い）。
func TestNewOverlayReplaysFullStateOnRegister(t *testing.T) {
	const kindEarly ModalKind = "early"

	o, _ := NewOverlay(testTab, state(120, 40))
	if cmd := o.Register(kindEarly, newStub("構築直後")); cmd == nil {
		t.Fatal("登録が Cmd を返していない（前提が崩れている）")
	}

	early := stubOf(t, o, kindEarly)
	if early.states == 0 {
		t.Fatal("構築時に登録したモーダルへ共有状態が配られていない")
	}

	got := early.state
	if n := len(got.Result.Runners); n != 1 || got.Result.Runners[0].Dir != testRunnerDir {
		t.Errorf("配られた検出結果 = %+v, want %q 1 台", got.Result, testRunnerDir)
	}
	if !got.Caps.Systemd || got.Caps.SudoUser == "" {
		t.Errorf("配られた能力 = %+v, want NewOverlay に渡した値", got.Caps)
	}
	if got.Exec == nil {
		t.Error("配られた共有状態に Executor が無い（ドメイン層を呼ぶ道が渡っていない）")
	}
	if got.Keys.Global.Help.Help().Key == "" {
		t.Error("配られた共有状態にキー定義が無い")
	}
	if got.BodyW != 120 || got.BodyH != 40 {
		t.Errorf("配られた本体領域 = %dx%d, want 120x40", got.BodyW, got.BodyH)
	}
	if early.size.W == 0 || early.size.W >= 120 {
		t.Errorf("配られた領域 = %+v, want 枠の分を引いた値", early.size)
	}
	if early.tab != testTab {
		t.Errorf("配られたタブ番号 = %d, want %d", early.tab, testTab)
	}
}
