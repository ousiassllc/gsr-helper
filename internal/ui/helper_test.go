package ui

import (
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/appconfig"
	"github.com/ousiassllc/gsr-helper/internal/doctor"
	"github.com/ousiassllc/gsr-helper/internal/exec"
	"github.com/ousiassllc/gsr-helper/internal/ui/chrome"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/pagetest"
)

// 内部テスト（package ui）にしてある。タブのメタ情報・tickMsg・chrome が非公開で、
// タブを差し替えて共有状態の配布を見るには内側へ触る必要があるためである。
//
// 共通の道具は page/pagetest から取る（書き写すと前提が食い違い、行数も増える）。

// press はキー入力の Msg を作る。
func press(k string) tea.KeyPressMsg { return pagetest.Press(k) }

// testCaps はすべての能力がある状態。
func testCaps() appconfig.Caps { return pagetest.Caps() }

// newApp は親 Model を組み立てる。走査ルートを空にして検出の入力を最小にする。
func newApp(ex exec.Executor) App {
	a := New(appconfig.Default(), testCaps(), ex, Options{
		Color:   false,
		Refresh: 2 * time.Second,
		Roots:   nil,
		Host:    "build01",
	})
	// **起動時の前提チェック（FR-44）は既定で走らせない。** 本物の項目は実ホストの
	// sudo / docker / /etc/group を読むため、親 Model の検証が実行環境の構成で
	// 揺れる。FR-44 そのものを見るテストは withHostChecks で差し替える。
	a.hostChecks = nil
	return a
}

// withHostChecks は起動時の前提チェックを差し替えた App を返す。
func withHostChecks(a App, checks ...doctor.Check) App {
	a.hostChecks = checks
	return a
}

// withSpies は有効なタブを pagetest.Spy に差し替える。並びはタブの並びと同じ。
//
// **差し戻し（Bubble）を立てる。** 親はグローバルキーを page が差し戻してきた
// ときにだけ解釈する（keys.go の配送）。立てないとキーの配送を検証できない。
func withSpies(a App) (App, []*pagetest.Spy) {
	spies := make([]*pagetest.Spy, 0, len(a.tabs))
	for i := range a.tabs {
		if !a.tabs[i].Enabled {
			continue
		}
		s := pagetest.NewSpy(i)
		s.Bubble = true
		s.Chrome = pageChrome(i)
		a.tabs[i].Model = s
		spies = append(spies, s)
	}
	return a, spies
}

// applyChrome は Cmd に含まれる ChromeMsg を親へ渡し、フッタを反映した App を返す。
//
// フッタは page が ChromeMsg で報告したものを親が描くため、フッタの表示を検証するには
// page → 親の 1 往復が必要である。取り出しは pagetest.ChromeMsgs が持つ。
func applyChrome(a App, cmd tea.Cmd) App {
	for _, msg := range pagetest.ChromeMsgs(cmd) {
		a, _ = update(a, msg)
	}
	return a
}

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
	if _, global, ok := pagetest.ScanKey(cmd); ok {
		return update(a, global)
	}
	return a, cmd
}

// statusLine は親が描く状態行を返す。
//
// 組み立ては chrome の純粋関数が持ち、親は値を写すだけである（chrome.go）。
func statusLine(a App) string { return chrome.Status(a.chromeView()) }
