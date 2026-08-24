package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/exec"
	"github.com/ousiassllc/gsr-helper/internal/ui/discovery"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/pagetest"
)

// ctrl+c はどの状態でも親が処理して終了する。
func TestInterruptQuitsInEveryState(t *testing.T) {
	states := blocked()
	states["通常"] = func(*pagetest.Spy) {}

	for name, setup := range states {
		t.Run(name, func(t *testing.T) {
			a, spies := withSpies(newApp(exec.NewFake()))
			setup(spies[0])
			_, cmd := update(a, press("ctrl+c"))
			if cmd == nil {
				t.Fatal("ctrl+c で Cmd が発行されない")
			}
			// 終了は後始末を流し切ってから行うため tea.Sequence に包まれる
			// （page.ShutdownMsg の doc）。
			if !isQuit(t, cmd) {
				t.Errorf("ctrl+c の Msg = %T, want 終了を含む Cmd", cmd())
			}
			if len(spies[0].Keys()) != 0 {
				t.Error("ctrl+c を page へ渡している")
			}
		})
	}
}

// モーダル表示中と入力中は、グローバルキーを解釈せず有効タブへ流す。
//
// page が差し戻さない（page.BubbleKey を呼ばない）ことでキーが閉じ込められる。
func TestGlobalKeysAreNotInterpretedWhenBlocked(t *testing.T) {
	for name, setup := range blocked() {
		t.Run(name, func(t *testing.T) {
			for _, k := range []string{"1", "2", "7", "tab", "shift+tab", "r", "q"} {
				a, spies := withSpies(newApp(exec.NewFake()))
				setup(spies[0])

				next, cmd := sendKey(a, k)
				if isQuit(t, cmd) {
					t.Errorf("キー %q で終了している", k)
				}
				if next.active != 0 {
					t.Errorf("キー %q でタブが %d に変わっている", k, next.active)
				}
				if len(spies[0].Keys()) != 1 {
					t.Errorf("キー %q が有効タブへ渡っていない", k)
				}
				if len(spies[1].Keys()) != 0 {
					t.Errorf("キー %q が無効タブへも渡っている", k)
				}
			}
		})
	}
}

// 通常時のグローバルキーは親が処理する。
func TestGlobalKeys(t *testing.T) {
	tests := map[string]struct {
		keys       []string
		wantActive int
		wantQuit   bool
	}{
		"番号キーでタブを選ぶ":        {[]string{"2"}, 1, false},
		"tab で次のタブへ":        {[]string{"tab"}, 1, false},
		"同じタブの番号は無視する":      {[]string{"1"}, 0, false},
		"q で終了する":           {[]string{"q"}, 0, true},
		"? は page が処理する":    {[]string{"?"}, 0, false},
		"esc は page が処理する":  {[]string{"esc"}, 0, false},
		"一覧のキーは page が処理する": {[]string{"j"}, 0, false},
		"操作キーは page が処理する":  {[]string{"x"}, 0, false},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			a, spies := withSpies(newApp(exec.NewFake()))
			var cmd tea.Cmd
			for _, k := range tt.keys {
				a, cmd = sendKey(a, k)
			}

			if a.active != tt.wantActive {
				t.Errorf("有効タブ = %d, want %d", a.active, tt.wantActive)
			}
			if got := isQuit(t, cmd); got != tt.wantQuit {
				t.Errorf("終了したか = %v, want %v", got, tt.wantQuit)
			}
			// どのキーもまず page へ渡る（親が先に解釈しない。keys.go の handleKey）。
			if len(spies[0].Keys()) != len(tt.keys) {
				t.Errorf("page へ渡ったキー = %d 件, want %d 件", len(spies[0].Keys()), len(tt.keys))
			}
		})
	}
}

// タブの移動が端で折り返す解決そのもの（Next）と、存在しない番号キーの解決
// （IndexOfKey が ok を偽で返す）は tabset.TestNextWrapsAtEnds が []Tab に対して
// 直接検証する。ここに残すのは、その解決が実際に打鍵から呼ばれ、page への配送
// （gate_test / route_test）やタブ切り替えの副作用（TestSwitchTabRefreshesChrome）と
// つながっていることを見るテストだけである。

// 無効なタブの番号キーは「効かないキー」にせず、無効である理由を状態行に出す。
func TestDisabledTabNumberShowsReason(t *testing.T) {
	a, spies := withSpies(newApp(exec.NewFake()))
	a, _ = update(a, tea.WindowSizeMsg{Width: 100, Height: 24})

	target := -1
	for i := range a.tabs {
		if !a.tabs[i].Enabled {
			target = i
			break
		}
	}
	if target < 0 {
		// 7 枚すべて実装済みなので、能力で無効にする枠のために 1 枚を無効にする。
		target = len(a.tabs) - 1
		a.tabs[target].Enabled = false
		a.tabs[target].Model = nil
		a.tabs[target].Reason = page.ReasonUnsupported
	}

	next, _ := sendKey(a, a.tabs[target].Key)
	if next.active != 0 {
		t.Errorf("無効なタブへ移っている（active = %d）", next.active)
	}
	for _, want := range []string{a.tabs[target].Title, a.tabs[target].Reason} {
		if !strings.Contains(statusLine(next), want) {
			t.Errorf("状態行 = %q, %q を含まない", statusLine(next), want)
		}
	}
	// 番号キーも page を経由する（親が先に解釈しない）。一覧は数字を使わないため、
	// 押した番号が一覧の操作として解釈されることはない。
	if len(spies[0].Keys()) != 1 {
		t.Errorf("無効なタブの番号キーが page へ渡っていない（%d 件）", len(spies[0].Keys()))
	}

	// 次の打鍵で案内は消える（状態行に残り続けない）。
	if after, _ := sendKey(next, "j"); strings.Contains(statusLine(after), a.tabs[target].Reason) {
		t.Errorf("次の打鍵の後の状態行 = %q, 案内が残っている", statusLine(after))
	}
}

// r は Tick を待たずに検出の Cmd を発行する。
func TestRefreshKeyEmitsDiscover(t *testing.T) {
	fake := exec.NewFake()
	a, spies := withSpies(newApp(fake))

	_, cmd := sendKey(a, "r")
	if cmd == nil {
		t.Fatal("r で Cmd が発行されない")
	}
	if _, ok := cmd().(discovery.Msg); !ok {
		t.Errorf("r の Msg = %T, want discovery.Msg", cmd())
	}
	if len(spies[0].Keys()) != 1 {
		t.Errorf("r が page へ渡っていない（%d 件）", len(spies[0].Keys()))
	}
}

// 無効なタブを tab で飛ばす解決そのもの（Next が無効なタブを飛ばす）と、番号キーが
// 無効なタブでも添字を引ける（IndexOfKey は有効・無効を見ない）ことは
// tabset.TestNextSkipsDisabledTabs が []Tab に対して直接検証する。

// タブを切り替えたら、新しいタブへ共有状態を配って ChromeMsg を促す。
func TestSwitchTabRefreshesChrome(t *testing.T) {
	a, spies := withSpies(newApp(exec.NewFake()))
	a.chrome.Modal, a.chrome.Input = false, ""

	a, cmd := sendKey(a, "2")
	if a.active != 1 {
		t.Fatalf("有効タブ = %d, want 1", a.active)
	}
	if len(spies[1].States()) != 1 {
		t.Errorf("切り替え先へ配られた StateMsg = %d 件, want 1", len(spies[1].States()))
	}
	if a.chrome.Tab != 1 {
		t.Errorf("切り替え直後の chrome のタブ = %d, want 1", a.chrome.Tab)
	}
	if cmd == nil {
		t.Fatal("切り替えで Cmd が発行されない")
	}
	// 切り替えは離脱・活性化・共有状態の 3 本を束ねて返すため、束の中から探す。
	found := false
	for _, c := range expand(t, cmd) {
		if got, ok := c().(page.ChromeMsg); ok && got.Tab == 1 {
			found = true
		}
	}
	if !found {
		t.Errorf("切り替えの Cmd に移動先タブの ChromeMsg が無い（%T）", cmd())
	}
}
