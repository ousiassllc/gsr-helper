package ui

import (
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/appconfig"
	"github.com/ousiassllc/gsr-helper/internal/exec"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/pagetest"
)

// テストは内部テスト（package ui）にしてある。タブのメタ情報・tickMsg・chrome は
// 非公開であり、タブを差し替えて「共有状態が全 page に配られるか」を見るために
// 内側へ触る必要があるためである。
//
// 共通の道具（キー入力の組み立て・能力・Cmd の展開・長寿命の処理を持つ page）は
// page/pagetest から取る。ここへ書き写すと、親と page で検証の前提が食い違ううえ、
// ui 直下の行数（1 ディレクトリ 2000 行）を道具立てで押し上げることになる。

// press はキー入力の Msg を作る。
func press(k string) tea.KeyPressMsg { return pagetest.Press(k) }

// testCaps はすべての能力がある状態。
func testCaps() appconfig.Caps { return pagetest.Caps() }

// newApp は親 Model を組み立てる。走査ルートを空にして検出の入力を最小にする。
func newApp(ex exec.Executor) App {
	return New(appconfig.Default(), testCaps(), ex, Options{
		Color:   false,
		Refresh: 2 * time.Second,
		Roots:   nil,
		Host:    "build01",
	})
}

// spy は共有状態とキーの受信を記録するテスト用の page。
//
// tea.Model をポインタで実装するのは、Update が返す値ではなく記録そのものを
// テストから見たいためである。
type spy struct {
	tab    int
	states []page.StateMsg
	keys   []tea.KeyPressMsg
	msgs   []tea.Msg // StateMsg / キー以外に受け取った Msg（page が発行した Cmd の結果）
	chrome page.ChromeMsg
}

// Init は何も発行しない。
func (s *spy) Init() tea.Cmd { return nil }

// Update は受け取った Msg を記録し、自分の ChromeMsg を返す。
//
// キーは本物の page と同じ規則で扱う。すなわち、モーダル表示中・入力中
// （chrome の Modal / Input）は親へ差し戻さず、それ以外は差し戻す
// （page.GlobalKeyMsg の doc）。差し戻さないと親はグローバルキーを解釈できない。
func (s *spy) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	bubble := false
	switch m := msg.(type) {
	case page.StateMsg:
		s.states = append(s.states, m)
	case tea.KeyPressMsg:
		s.keys = append(s.keys, m)
		bubble = !s.chrome.Modal && s.chrome.Input == ""
	default:
		s.msgs = append(s.msgs, m)
	}

	c := s.chrome
	c.Tab = s.tab
	chrome := func() tea.Msg { return c }
	if press, ok := msg.(tea.KeyPressMsg); ok && bubble {
		return s, tea.Batch(chrome, page.BubbleKey(press))
	}
	return s, chrome
}

// View は本体の代わりにタブ番号を返す。
func (s *spy) View() tea.View { return tea.NewView("spy") }

// withSpies は有効なタブをテスト用の page に差し替える。
//
// 無効なタブ（この版で未実装のタブ）は Model を持たないため差し替えない。返す
// spy の並びはタブの並びと同じで、添字 0 が Runners、1 が Jobs である。
func withSpies(a App) (App, []*spy) {
	spies := make([]*spy, 0, len(a.tabs))
	for i := range a.tabs {
		if !a.tabs[i].Enabled {
			continue
		}
		s := &spy{tab: i, states: nil, keys: nil, msgs: nil, chrome: pageChrome(i)}
		a.tabs[i].Model = s
		spies = append(spies, s)
	}
	return a, spies
}

// lastEnabledTab は最後の有効なタブの添字を返す。
func lastEnabledTab(tabs []tab) int {
	last := -1
	for i := range tabs {
		if tabs[i].Enabled {
			last = i
		}
	}
	return last
}

// chromeOf は Cmd に含まれる ChromeMsg を親へ渡し、フッタを反映した App を返す。
//
// フッタは page が ChromeMsg で報告したものを親が描くため、フッタの表示を検証するには
// page → 親の 1 往復が必要である。
func applyChrome(a App, cmd tea.Cmd) App {
	for _, c := range cmdList(cmd) {
		if c == nil {
			continue
		}
		if msg, ok := c().(page.ChromeMsg); ok {
			a, _ = update(a, msg)
		}
	}
	return a
}

// cmdList は Batch / Sequence の Cmd を展開して返す。中の Cmd は実行しない。
func cmdList(cmd tea.Cmd) []tea.Cmd { return pagetest.Expand(cmd) }

// asCmds は Msg が Cmd の並び（Batch / Sequence）ならその中身を返す。
//
// **Batch と Sequence は区別できない**（pagetest.Cmds の doc）。順序そのものを
// 検証する側は Msg の型で判別すること（lifecycle_test.go の終了の検証）。
func asCmds(msg tea.Msg) ([]tea.Cmd, bool) { return pagetest.Cmds(msg) }

// update は Msg を 1 つ渡し、App と Cmd を返す。
func update(a App, msg tea.Msg) (App, tea.Cmd) {
	m, cmd := a.Update(msg)
	next, ok := m.(App)
	if !ok {
		panic("Update が App 以外を返した")
	}
	return next, cmd
}

// sendKey は打鍵を渡し、page が差し戻したグローバルキーまで解釈させる。
//
// 親はキーを必ず有効タブへ渡し、page が自分では使わないキーだけを
// page.GlobalKeyMsg として差し戻す（keys.go の配送）。実機ではこの往復が bubbletea の
// Msg ループで起きるため、テストでも同じ順で回す。返す Cmd は差し戻しを処理した
// 結果のもの（差し戻しが無ければ打鍵そのものの結果）である。
func sendKey(a App, k string) (App, tea.Cmd) {
	a, cmd := update(a, press(k))
	for _, c := range cmdList(cmd) {
		if c == nil {
			continue
		}
		if msg, ok := c().(page.GlobalKeyMsg); ok {
			return update(a, msg)
		}
	}
	return a, cmd
}
