package ui

import (
	"reflect"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/exec"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/pagetest"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/pagetest/cmdtest"
)

// page の寿命の通知（Issue #41）を検証する。裏へ回ったこと・前面に戻ったこと・
// 終了することが page へ届き、後始末の Cmd が終了より前に流れることを見る。
//
// 長寿命の購読を持つ page は pagetest.StreamPage を使う（前面で 1 本張り、裏へ
// 回ったら畳む）。

// タブを往復しても購読が積み上がらない。
//
// 離れるタブへ page.DeactivateMsg が届かないと、裏に回った page は畳む機会が無く、
// 往復のたびに購読が 1 本ずつ増える（Issue #41 の症状）。
func TestTabRoundTripDoesNotAccumulateSubscriptions(t *testing.T) {
	const trips = 3

	a, pages := withStreams(newApp(exec.NewFake()))

	for range trips {
		a, _ = sendKey(a, "2")
		if a.active != 1 {
			t.Fatalf("タブ 1 へ移っていない（active = %d）", a.active)
		}
		if got := pages[1].Open; got != 1 {
			t.Fatalf("前面へ出たタブの購読 = %d 本, want 1", got)
		}
		a, _ = sendKey(a, "1")
		if a.active != 0 {
			t.Fatalf("タブ 0 へ戻っていない（active = %d）", a.active)
		}
	}

	if got := pages[1].Open; got != 0 {
		t.Errorf("裏のタブの購読 = %d 本, want 0", got)
	}
	if got := pages[1].Peak; got != 1 {
		t.Errorf("同時購読の最大 = %d 本, want 1（往復で積み上がっている）", got)
	}
}

// 終了では有効な全タブへ通知が届き、後始末の Cmd が終了より前に流れる。
func TestQuitRunsPageCleanupBeforeQuit(t *testing.T) {
	a, pages := withStreams(newApp(exec.NewFake()))

	// タブ 1 を前面に出して購読を張らせた状態で終了する。
	a, _ = sendKey(a, "2")
	if pages[1].Open != 1 {
		t.Fatal("前面のタブが購読を張っていない（前提が崩れている）")
	}

	_, cmd := update(a, press("ctrl+c"))
	if cmd == nil {
		t.Fatal("終了で Cmd が発行されない")
	}

	// **束ね方そのものを見る。** tea.Batch では後始末と終了が並走し、後始末が
	// 実行される前にランタイムが止まりうる（page.ShutdownMsg の doc）。中身は
	// どちらも []tea.Cmd なので、展開して数えるだけでは区別できない。
	msg := cmd()
	if _, batch := msg.(tea.BatchMsg); batch {
		t.Fatal("終了の Cmd が tea.Batch である（後始末が終了と並走する）")
	}
	if got := reflect.TypeOf(msg).String(); got != "tea.sequenceMsg" {
		t.Fatalf("終了の Cmd の Msg = %s, want tea.sequenceMsg（tea.Sequence で束ねる）", got)
	}

	steps, ok := cmdtest.Cmds(msg)
	if !ok || len(steps) < 2 {
		t.Fatalf("終了の Cmd = %d 本, want 後始末と終了の 2 本以上", len(steps))
	}

	// 最後が終了であること（後始末より先に止まらない）。
	last := steps[len(steps)-1]
	if _, ok := last().(tea.QuitMsg); !ok {
		t.Fatalf("終了の Cmd の最後 = %T, want tea.QuitMsg", last())
	}

	// 終了より前の Cmd を流すと、全タブの後始末が実行される。
	for _, c := range steps[:len(steps)-1] {
		cmdtest.RunAll(c)
	}
	// 裏のタブにも通知は届く（畳み損ねた処理をここで確実に閉じられる）。
	for i, p := range pages {
		if p.Shutdowns != 1 {
			t.Errorf("タブ %d が受けた終了の通知 = %d 回, want 1", i, p.Shutdowns)
		}
		if p.Open != 0 {
			t.Errorf("タブ %d の購読 = %d 本, want 0（終了で畳まれていない）", i, p.Open)
		}
	}
	if pages[1].Stops != 1 {
		t.Errorf("前面のタブの後始末 = %d 回, want 1", pages[1].Stops)
	}
}

// 起動時に選択されているタブが page.ActivateMsg を 1 度だけ受け取る（Issue #63）。
//
// 受け取らないと、既定タブに長寿命の処理を持つ page を置いた瞬間に、その処理が
// 黙って張られないままになる。逆に共有状態のたびに配ると購読が積み上がる。
func TestInitialTabIsActivatedExactlyOnce(t *testing.T) {
	a, pages := withStreams(newApp(exec.NewFake()))

	// 共有状態は起動直後に何度も配られる（端末サイズ・背景色・再検出）。
	a, _ = update(a, tea.WindowSizeMsg{Width: 80, Height: 24})
	a, _ = update(a, tea.BackgroundColorMsg{})
	a, _ = update(a, tea.WindowSizeMsg{Width: 100, Height: 30})

	if got := pages[a.active].Open; got != 1 {
		t.Errorf("既定タブの購読 = %d 本, want 1（起動時の前面化が 1 度だけ届く）", got)
	}
	if got := pages[a.active].Peak; got != 1 {
		t.Errorf("既定タブの同時購読の最大 = %d 本, want 1（配るたびに積み上がっている）", got)
	}
	for i, p := range pages {
		if i == a.active {
			continue
		}
		if p.Open != 0 {
			t.Errorf("裏のタブ %d に前面化が届いている（購読 %d 本）", i, p.Open)
		}
	}
}

// 起動時に前面化を受け取ったタブから離れて戻っても、購読は積み上がらない。
//
// tabset.ActivateOnce が「1 度だけ」を覚えることと、activate が配る往復ぶんの
// 前面化・非活性化が対になっていることの両方を見る。
func TestInitialActivationDoesNotDoubleCountOnReturn(t *testing.T) {
	a, pages := withStreams(newApp(exec.NewFake()))
	a, _ = update(a, tea.WindowSizeMsg{Width: 80, Height: 24})

	home := a.active
	a, _ = sendKey(a, "2")
	a, _ = sendKey(a, "1")

	if got := pages[home].Open; got != 1 {
		t.Errorf("往復後の購読 = %d 本, want 1", got)
	}
	if got := pages[home].Peak; got != 1 {
		t.Errorf("同時購読の最大 = %d 本, want 1（起動時と復帰で二重に張っている）", got)
	}
}

// 起動シーケンスの外へ回した取得の駆動（Issue #73 / #79）を検証する。

// _work の集計は再検出のたびに走らない。
//
// 駆動の契機は「最初の検出成功」と手動の再読み込みだけである。検出が成功するたびに
// 走らせると、runner 1 台で秒〜分かかる走査が 3 秒ごとに始まり、既定タブの応答を壊す。
// **実行中でないときに次の周期が来る**のは普通に起こるので、重複防止だけでは足りない。
func TestWorkScanRunsOnceAcrossDiscoveryCycles(t *testing.T) {
	a, _ := update(newApp(exec.NewFake()), tea.WindowSizeMsg{Width: 80, Height: 24})

	if got := pagetest.WorkScanStarts(a, t.TempDir(), 3); got != 1 {
		t.Errorf("集計の発行 = %d 回, want 1（再検出のたびに走っている）", got)
	}
}
