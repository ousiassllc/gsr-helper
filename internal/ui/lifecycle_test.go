package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/exec"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
)

// page の寿命の通知（Issue #41）を検証する。裏へ回ったこと・前面に戻ったこと・
// 終了することが page へ届き、後始末の Cmd が終了より前に流れることを見る。

// cleanupDoneMsg は後始末の Cmd が実行されたことを表す。
type cleanupDoneMsg struct{ tab int }

// streamPage は長寿命の購読（journalctl -f のようなストリーム）を持つ page を
// 模したテスト用の Model。
//
// 前面に出たら購読を 1 本張り、裏へ回ったら畳む。終了では残っているものを畳む。
// 起動時に選択されているタブは page.ActivateMsg を受け取らない（同 doc）ため、
// 購読は 0 本から始める。
type streamPage struct {
	tab       int
	open      int // 開いている購読の本数
	peak      int // 同時に開いた最大本数（積み上がりの検出に使う）
	stops     int // 後始末の Cmd が実行された回数
	shutdowns int // 終了の通知を受けた回数
}

// tea.Model を実装していることをコンパイル時に確かめる。
var _ tea.Model = (*streamPage)(nil)

// newStreamPage は購読をまだ張っていない page を返す。
func newStreamPage(tab int) *streamPage {
	return &streamPage{tab: tab, open: 0, peak: 0, stops: 0, shutdowns: 0}
}

func (s *streamPage) Init() tea.Cmd { return nil }

// Update は寿命の通知で購読を張り直し、キーは親へ差し戻す。
func (s *streamPage) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case page.ActivateMsg:
		s.open++
		s.peak = max(s.peak, s.open)
		return s, nil
	case page.DeactivateMsg:
		return s, s.close()
	case page.ShutdownMsg:
		s.shutdowns++
		return s, s.close()
	case tea.KeyPressMsg:
		return s, page.BubbleKey(msg)
	default:
		return s, nil
	}
}

func (s *streamPage) View() tea.View { return tea.NewView("stream") }

// close は購読を 1 本畳み、その後始末の Cmd を返す。開いていなければ何もしない。
func (s *streamPage) close() tea.Cmd {
	if s.open == 0 {
		return nil
	}
	s.open--
	tab := s.tab
	return func() tea.Msg {
		s.stops++
		return cleanupDoneMsg{tab: tab}
	}
}

// withStreams は有効なタブを streamPage に差し替える。
func withStreams(a App) (App, []*streamPage) {
	pages := make([]*streamPage, 0, len(a.tabs))
	for i := range a.tabs {
		if !a.tabs[i].Enabled {
			continue
		}
		p := newStreamPage(i)
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
	if len(pages) < 2 {
		t.Fatal("有効なタブが 2 枚未満（前提が崩れている）")
	}

	for range trips {
		a, _ = sendKey(a, "2")
		if a.active != 1 {
			t.Fatalf("タブ 1 へ移っていない（active = %d）", a.active)
		}
		if got := pages[1].open; got != 1 {
			t.Fatalf("前面へ出たタブの購読 = %d 本, want 1", got)
		}
		a, _ = sendKey(a, "1")
		if a.active != 0 {
			t.Fatalf("タブ 0 へ戻っていない（active = %d）", a.active)
		}
	}

	if got := pages[1].open; got != 0 {
		t.Errorf("裏のタブの購読 = %d 本, want 0", got)
	}
	if got := pages[1].peak; got != 1 {
		t.Errorf("同時購読の最大 = %d 本, want 1（往復で積み上がっている）", got)
	}
}

// 終了では有効な全タブへ通知が届き、後始末の Cmd が終了より前に流れる。
func TestQuitRunsPageCleanupBeforeQuit(t *testing.T) {
	a, pages := withStreams(newApp(exec.NewFake()))

	// タブ 1 を前面に出して購読を張らせた状態で終了する。
	a, _ = sendKey(a, "2")
	if pages[1].open != 1 {
		t.Fatal("前面のタブが購読を張っていない（前提が崩れている）")
	}

	_, cmd := update(a, press("ctrl+c"))
	steps := cmdList(cmd)
	if len(steps) < 2 {
		t.Fatalf("終了の Cmd = %d 本, want 後始末と終了の 2 本以上", len(steps))
	}

	// 最後が終了であること（後始末より先に止まらない）。
	last := steps[len(steps)-1]
	if _, ok := last().(tea.QuitMsg); !ok {
		t.Fatalf("終了の Cmd の最後 = %T, want tea.QuitMsg", last())
	}

	// 終了より前の Cmd を流すと、全タブの後始末が実行される。
	for _, c := range steps[:len(steps)-1] {
		runAll(c)
	}
	// 裏のタブにも通知は届く（畳み損ねた処理をここで確実に閉じられる）。
	for i, p := range pages {
		if p.shutdowns != 1 {
			t.Errorf("タブ %d が受けた終了の通知 = %d 回, want 1", i, p.shutdowns)
		}
		if p.open != 0 {
			t.Errorf("タブ %d の購読 = %d 本, want 0（終了で畳まれていない）", i, p.open)
		}
	}
	if pages[1].stops != 1 {
		t.Errorf("前面のタブの後始末 = %d 回, want 1", pages[1].stops)
	}
}

// runAll は Cmd を 1 度だけ実行し、結果が Cmd の並びならその中身も実行する。
func runAll(c tea.Cmd) {
	if c == nil {
		return
	}
	inner, ok := asCmds(c())
	if !ok {
		return
	}
	for _, ic := range inner {
		runAll(ic)
	}
}
