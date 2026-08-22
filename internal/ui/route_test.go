package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/exec"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/pagetest"
)

// domainResult は page が発行したドメイン呼び出しの結果に相当するテスト用の Msg。
type domainResult struct{ n int }

// page が発行した Cmd の結果は、届く前にタブを切り替えても発行元のタブへ渡る。
//
// 選択中のタブへ配ると、裏になったタブは自分で始めた処理を完了できず結果が失われる
// （後続の全タブが非同期のドメイン呼び出しを持つ。page.TabMsg の doc）。
func TestTabMsgGoesBackToIssuingTab(t *testing.T) {
	a, spies := withSpies(newApp(exec.NewFake()))

	// タブ 0（Runners）がドメイン層を呼び、結果が届く前に利用者がタブ 1 へ移る。
	cmd := page.Do(0, func() tea.Msg { return domainResult{n: 7} })
	a, _ = sendKey(a, "2")
	if a.active != 1 {
		t.Fatalf("タブ 1 に移っていない（active = %d）", a.active)
	}

	a, _ = update(a, cmd())

	if got := received(spies[0]); len(got) != 1 || got[0] != (domainResult{n: 7}) {
		t.Errorf("発行元のタブ 0 が受け取った Msg = %v, want [domainResult{7}]", got)
	}
	if got := received(spies[1]); len(got) != 0 {
		t.Errorf("別のタブ 1 に結果が渡っている（%v）", got)
	}
}

// 無効になったタブ宛の結果は捨てる（配る先の Model が無い）。
//
// **他のタブへ回さないことまで見る。** Cmd が nil であることだけを見ていた頃は、
// 結果がタブ 0 へ誤配送されても緑のままだった（Issue #31）。捨てるべき Msg が別の
// タブへ入ると、そのタブは自分が始めていない処理の結果で状態を書き換える。
func TestTabMsgForDeadTabIsDropped(t *testing.T) {
	a, spies := withSpies(newApp(exec.NewFake()))

	// タブ 2（Disk）はこの版では Model を持たない。
	if _, cmd := update(a, page.TabMsg{Tab: 2, Msg: domainResult{n: 1}}); cmd != nil {
		t.Errorf("無効タブ宛の結果で Cmd が発行された（%T）", cmd)
	}
	for i, s := range spies {
		if got := received(s); len(got) != 0 {
			t.Errorf("無効タブ宛の結果が有効なタブ %d へ配られた（%v）", i, got)
		}
	}
}

// received は spy が受け取った domainResult を並び順に返す。
func received(s *pagetest.Spy) []domainResult {
	out := make([]domainResult, 0, len(s.Msgs()))
	for _, m := range s.Msgs() {
		if r, ok := m.(domainResult); ok {
			out = append(out, r)
		}
	}
	return out
}

// 共有状態は Executor を全タブへ配る。
//
// ドメイン層を tea.Cmd で呼べるのは page 階層だけであり（atomic-design.md の依存の
// 規則）、その page へ Executor を渡す道はこの Msg しかない。配らないと、操作を実装する
// 後続 Issue ごとに StateMsg と親 Model の両方を直すことになる。
//
// **systemctl が無い環境でも nil にしない。** 検出だけが nil にして systemd の参照を
// 落とす縮退を持つ（discover.go の discoverExec）が、それは runner.Discover の契約で
// あって page の約束ではない。
func TestStateCarriesExecutorToEveryTab(t *testing.T) {
	fake := exec.NewFake()
	a, spies := withSpies(newApp(fake))
	a.caps.Systemd = false // systemctl が無い環境でも配る

	a, _ = update(a, tea.WindowSizeMsg{Width: 100, Height: 30})

	for i, s := range spies {
		if len(s.States()) == 0 {
			t.Fatalf("タブ %d に共有状態が配られていない", i)
		}
		if got := s.States()[len(s.States())-1].Exec; got != fake {
			t.Errorf("タブ %d が受け取った Executor = %v, want 起動時のもの", i, got)
		}
	}
}
