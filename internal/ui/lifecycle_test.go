package ui

import (
	"reflect"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/exec"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/pagetest"
)

// page の寿命の通知（Issue #41）を検証する。裏へ回ったこと・前面に戻ったこと・
// 終了することが page へ届き、後始末の Cmd が終了より前に流れることを見る。
//
// 長寿命の購読を持つ page は pagetest.StreamPage を使う（前面で 1 本張り、裏へ
// 回ったら畳む）。

// withStreams は有効なタブを pagetest.StreamPage に差し替える。
func withStreams(a App) (App, []*pagetest.StreamPage) {
	pages := make([]*pagetest.StreamPage, 0, len(a.tabs))
	for i := range a.tabs {
		if !a.tabs[i].Enabled {
			continue
		}
		p := pagetest.NewStreamPage(i)
		a.tabs[i].Model = p
		pages = append(pages, p)
	}
	return a, pages
}

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

	steps, ok := asCmds(msg)
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
		pagetest.RunAll(c)
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
