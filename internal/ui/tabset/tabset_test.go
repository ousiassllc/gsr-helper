package tabset

import (
	"strconv"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/appconfig"
	"github.com/ousiassllc/gsr-helper/internal/exec"
	"github.com/ousiassllc/gsr-helper/internal/runner"
	"github.com/ousiassllc/gsr-helper/internal/ui/keymap"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/pagetest"
	"github.com/ousiassllc/gsr-helper/internal/ui/token"
)

// capsMatrix は能力の組み合わせを返す。タブの有効・無効は Caps から決まるため、
// 欠けている能力ごとに検証する。
func capsMatrix() map[string]appconfig.Caps {
	drops := map[string]func(*appconfig.Caps){
		"すべてある":      func(*appconfig.Caps) {},
		"非 root":     func(c *appconfig.Caps) { c.Root = false },
		"systemd なし": func(c *appconfig.Caps) { c.Systemd = false },
		"docker なし":  func(c *appconfig.Caps) { c.Docker = false },
		"journal なし": func(c *appconfig.Caps) { c.Journal = false },
		"gh 未認証":     func(c *appconfig.Caps) { c.GitHubToken = false },
		"何も使えない": func(c *appconfig.Caps) {
			*c = appconfig.Caps{}
		},
	}
	out := make(map[string]appconfig.Caps, len(drops))
	for name, drop := range drops {
		c := pagetest.Caps()
		drop(&c)
		out[name] = c
	}
	return out
}

// newTestTabs は能力からタブを組み立てる。
func newTestTabs(caps appconfig.Caps) []Tab {
	return New(caps, exec.NewFake(), keymap.New(), token.NewStyles(true, false), true)
}

// testState はタブへ配る共有状態のスナップショット。
func testState() page.StateMsg {
	return page.StateMsg{
		Result: runner.Result{},
		Caps:   pagetest.Caps(),
		Styles: token.NewStyles(true, false),
		Keys:   keymap.New(),
		Exec:   exec.NewFake(),
		Dark:   true,
		BodyW:  80,
		BodyH:  20,
		Err:    nil,
	}
}

// chromeTab は Cmd に含まれる ChromeMsg が名乗るタブ番号を返す。
func chromeTab(cmd tea.Cmd) (int, bool) {
	for _, c := range pagetest.Expand(cmd) {
		if c == nil {
			continue
		}
		if msg, ok := c().(page.ChromeMsg); ok {
			return msg.Tab, true
		}
	}
	return 0, false
}

// タブの番号キーは一意で、1 から連番になる（screens.md のグローバルキー）。
func TestTabKeysAreUniqueAndSequential(t *testing.T) {
	tabs := newTestTabs(pagetest.Caps())
	if len(tabs) == 0 {
		t.Fatal("タブが 1 枚も無い")
	}

	seen := make(map[string]bool, len(tabs))
	for i, tb := range tabs {
		if want := strconv.Itoa(i + 1); tb.Key != want {
			t.Errorf("%d 番目のタブのキー = %q, want %q", i, tb.Key, want)
		}
		if seen[tb.Key] {
			t.Errorf("番号キー %q が重複している", tb.Key)
		}
		seen[tb.Key] = true
		if tb.Title == "" {
			t.Errorf("%d 番目のタブに名前が無い", i)
		}
	}
}

// どの能力でも有効なタブの page は組み立てられる（能力不足は縮退で扱う）。
//
// 無効なタブ（この版で未実装のタブ）は Model を持たない。親はそこへキーも
// StateMsg も配らない（ui の live）。
func TestEnabledTabModelsAreNeverNil(t *testing.T) {
	for name, caps := range capsMatrix() {
		t.Run(name, func(t *testing.T) {
			for i, tb := range newTestTabs(caps) {
				switch {
				case tb.Enabled && tb.Model == nil:
					t.Errorf("%d 番目のタブ（%s）は有効なのに Model が nil である", i, tb.Title)
				case !tb.Enabled && tb.Model != nil:
					t.Errorf("%d 番目のタブ（%s）は無効なのに Model を持っている", i, tb.Title)
				}
			}
		})
	}
}

// タブ行には screens.md の共通レイアウトの 7 枚を出す。
//
// 未実装のタブも出すのは、押しても何も起きないキーを作らないためである。後続 Issue は
// 該当する 1 行を実装済みのタブに差し替えるだけで有効化できる。
func TestTabsCoverSpecLayout(t *testing.T) {
	want := []string{"Runners", "Jobs", "Disk", "Logs", "Doctor", "Config", "Setup"}
	tabs := newTestTabs(pagetest.Caps())
	if len(tabs) != len(want) {
		t.Fatalf("タブの枚数 = %d, want %d", len(tabs), len(want))
	}
	for i, title := range want {
		if tabs[i].Title != title {
			t.Errorf("%d 番目のタブ = %q, want %q", i, tabs[i].Title, title)
		}
	}
}

// 無効なタブには必ず理由を添える（TabBar がグレーアウトの理由を出すため）。
//
// 実装済みのタブ（Runners / Jobs）はホスト内の読み取りだけで成立するので、どの能力でも
// 無効にならない。未実装のタブは「この版では未対応です」を理由に持つ。
func TestDisabledTabHasReason(t *testing.T) {
	for name, caps := range capsMatrix() {
		t.Run(name, func(t *testing.T) {
			for _, tb := range newTestTabs(caps) {
				switch {
				case !tb.Enabled && tb.Reason == "":
					t.Errorf("タブ %s が無効なのに理由が無い", tb.Title)
				case tb.Enabled && tb.Reason != "":
					t.Errorf("タブ %s は有効なのに理由がある: %s", tb.Title, tb.Reason)
				}
			}
		})
	}
}

// 初期のスナップショットは能力とキー定義を持って各 page へ渡る。
//
// 最初のリサイズと検出が届く前でも、page が配色とキー定義を持った状態で描画できる
// ことを担保する。
func TestTabsGetInitialState(t *testing.T) {
	for _, tb := range newTestTabs(pagetest.Caps()) {
		if tb.Model == nil {
			continue
		}
		if v := tb.Model.View(); v.Content == "" {
			t.Errorf("タブ %s が初期状態で何も描けていない", tb.Title)
		}
	}
}

// 無効なタブの理由は未対応の操作と同じ文言を使う。
//
// 利用者にとって「無効なタブ」と「未対応の操作」は同じ意味（この版ではまだ使えない）
// なので、文言は 1 つの定数（page.ReasonUnsupported）から来なければならない。
func TestDisabledTabReasonIsShared(t *testing.T) {
	for _, tb := range newTestTabs(pagetest.Caps()) {
		if !tb.Enabled && tb.Reason != page.ReasonUnsupported {
			t.Errorf("タブ %s の理由 = %q, want %q", tb.Title, tb.Reason, page.ReasonUnsupported)
		}
	}
}

// 無効なタブの案内は番号キーと名前と理由を並べる（親が状態行に出す）。
func TestNoticeShowsKeyTitleAndReason(t *testing.T) {
	for _, tb := range newTestTabs(pagetest.Caps()) {
		if tb.Enabled {
			continue
		}
		if want := "[" + tb.Key + "]" + tb.Title + " " + tb.Reason; tb.Notice() != want {
			t.Errorf("タブ %s の案内 = %q, want %q", tb.Title, tb.Notice(), want)
		}
	}
}

// page が名乗るタブ番号は []Tab の添字と一致する。
//
// page は自分のタブ番号を ChromeMsg と TabMsg に載せ、親はそれを添字として突き合わせる
// （ui の ChromeMsg / TabMsg の分岐）。ずれても例外もログも出ず、フッタ・状態行・
// モーダルフラグが恒久的に更新されなくなり、page が発行した Cmd の結果は別のタブへ
// 配られるだけなので、ここで機械的に検出する。
func TestTabIndexMatchesPageTabNumber(t *testing.T) {
	tabs := newTestTabs(pagetest.Caps())
	st := testState()
	for i := range tabs {
		if tabs[i].Model == nil {
			continue
		}
		_, cmd := tabs[i].Model.Update(st)
		got, ok := chromeTab(cmd)
		if !ok {
			t.Errorf("%d 番目のタブ（%s）が ChromeMsg を返さない", i, tabs[i].Title)
			continue
		}
		if got != i {
			t.Errorf("タブ %s が名乗るタブ番号 = %d, want %d（[]Tab の添字）", tabs[i].Title, got, i)
		}
	}
}
