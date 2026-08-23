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

	if got := pagetest.MsgsOf[domainResult](spies[0]); len(got) != 1 || got[0] != (domainResult{n: 7}) {
		t.Errorf("発行元のタブ 0 が受け取った Msg = %v, want [domainResult{7}]", got)
	}
	if got := pagetest.MsgsOf[domainResult](spies[1]); len(got) != 0 {
		t.Errorf("別のタブ 1 に結果が渡っている（%v）", got)
	}
}

// 無効になったタブ宛の結果は捨てる（配る先の Model が無い）。
//
// **他のタブへ回さないことまで見る。** Cmd が nil であることだけを見ていた頃は、
// 結果がタブ 0 へ誤配送されても緑のままだった（Issue #31）。
func TestTabMsgForDeadTabIsDropped(t *testing.T) {
	a, spies := withSpies(newApp(exec.NewFake()))

	// Model を持たないタブを 1 枚作って「配る先が無い結果」を再現する。
	dead := len(a.tabs) - 1
	a.tabs[dead].Enabled, a.tabs[dead].Model = false, nil

	if _, cmd := update(a, page.TabMsg{Tab: dead, Msg: domainResult{n: 1}}); cmd != nil {
		t.Errorf("無効タブ宛の結果で Cmd が発行された（%T）", cmd)
	}
	for i, s := range spies {
		if got := pagetest.MsgsOf[domainResult](s); len(got) != 0 {
			t.Errorf("無効タブ宛の結果が有効なタブ %d へ配られた（%v）", i, got)
		}
	}
}

// 共有状態は Executor を全タブへ配る。
//
// ドメイン層を tea.Cmd で呼べるのは page 階層だけであり（atomic-design.md の依存の
// 規則）、その page へ Executor を渡す道はこの Msg しかない。配らないと、操作を実装する
// 後続 Issue ごとに StateMsg と親 Model の両方を直すことになる。
//
// **systemctl が無い環境でも nil にしない。** 検出だけが nil にして systemd の参照を
// 落とす縮退を持つ（discovery.Exec）が、それは runner.Discover の契約で
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

// タブをまたぐ移動は親が担う。移動先へ移り、用件をそのタブへ配る（page.OpenTabMsg）。
//
// タブ同士は互いを import しないため、移動元は移動先の番号も型も持てない。
// 名前とタブ番号の対応を知っているのは親だけである。
func TestOpenTabMovesAndDelivers(t *testing.T) {
	a, spies := withSpies(newApp(exec.NewFake()))
	spy := spyOfTitle(t, a, spies, page.TabLogs)

	a, _ = update(a, page.OpenTabMsg{Title: page.TabLogs, Msg: page.ShowLogMsg{}})

	if a.tabs[a.active].Title != page.TabLogs {
		t.Errorf("移動後のタブ = %q, want %q", a.tabs[a.active].Title, page.TabLogs)
	}
	if pagetest.Delivered[page.ShowLogMsg](spy) != 1 {
		t.Error("移動先へ用件が配られていない")
	}
}

// spyOfTitle は名前で spy を引く。spies の並びは有効なタブの順であり、タブの
// 添字とは一致しない（未実装のタブは差し替えないため）。
func spyOfTitle(t *testing.T, a App, spies []*pagetest.Spy, title string) *pagetest.Spy {
	t.Helper()

	for _, s := range spies {
		if a.tabs[s.Tab].Title == title {
			return s
		}
	}
	t.Fatalf("タブ %q の spy が無い（前提が崩れている）", title)
	return nil
}

// 既に前面に居るタブへの要求でも用件は配る（同じタブで l を押した場合）。
func TestOpenTabDeliversToActiveTab(t *testing.T) {
	a, spies := withSpies(newApp(exec.NewFake()))
	title := a.tabs[a.active].Title
	spy := spyOfTitle(t, a, spies, title)

	a, _ = update(a, page.OpenTabMsg{Title: title, Msg: page.ShowLogMsg{}})

	if pagetest.Delivered[page.ShowLogMsg](spy) != 1 {
		t.Error("前面に居るタブへ用件が配られていない")
	}
}

// Runners 一覧の n / D / u は Setup タブへ移す（screens.md の画面遷移）。
//
// **確認ダイアログは移動先が出す。** 移動元でも出すと同じ操作の確認が 2 箇所に
// 分かれ、起点によって中身が食い違いうる。打鍵が page.OpenTabMsg になり、親が
// それを解いて Setup が前面に出るまでを通しで見る。
func TestRunnerKeysOpenSetupTab(t *testing.T) {
	for _, k := range []string{"n", "D", "u"} {
		t.Run(k, func(t *testing.T) {
			f := exec.NewFake()
			a := newAppWithRunner(f)

			a, cmd := update(a, press(k))
			open, ok := pagetest.OpenTabOf(cmd)
			if !ok {
				t.Fatal("page.OpenTabMsg が発行されていない")
			}
			if open.Title != page.TabSetup {
				t.Errorf("移動先 = %q, want %q", open.Title, page.TabSetup)
			}

			a, _ = update(a, open)
			if got := a.tabs[a.active].Title; got != page.TabSetup {
				t.Errorf("%q を押した後のタブ = %q, want %q", k, got, page.TabSetup)
			}
			if len(f.Calls()) != 0 {
				t.Errorf("移動元がコマンドを発行している: %v", f.Calls())
			}
		})
	}
}
