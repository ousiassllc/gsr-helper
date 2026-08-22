package ui

import (
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/appconfig"
	"github.com/ousiassllc/gsr-helper/internal/exec"
	"github.com/ousiassllc/gsr-helper/internal/ui/chrome"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/pagetest"
	"github.com/ousiassllc/gsr-helper/internal/ui/tabset"
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

// withSpies は有効なタブを pagetest.Spy に差し替える。
//
// 無効なタブ（この版で未実装のタブ）は Model を持たないため差し替えない。返す
// spy の並びはタブの並びと同じで、添字 0 が Runners、1 が Jobs である。
//
// **差し戻し（Bubble）を立てる。** 親はグローバルキーを、page が「自分では使わない」
// と判断して差し戻してきたときにだけ解釈する（keys.go の配送）。立てないとキーの
// 配送そのものを検証できない。ui 直下はこの 1 つの振る舞いのために同じ spy を
// 写し持っていたが、pagetest.Spy の任意の振る舞いにして寄せた（Issue #45）。
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

// lastEnabledTab は最後の有効なタブの添字を返す。
func lastEnabledTab(tabs []tabset.Tab) int {
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
	if msg, ok := findBubbled(cmd); ok {
		return update(a, msg)
	}
	return a, cmd
}

// cmdWait は Cmd 1 つの実行を待つ上限。
//
// 探している Msg（page.ChromeMsg / page.GlobalKeyMsg / 束）を返す Cmd はどれも値を
// 組み立てて返すだけのクロージャであり、即座に返る。待たされるのは絞り込み中の一覧が
// 返すカーソル点滅（tea.Tick で約 0.5 秒）だけである。点滅より十分短く、かつ -race や
// CI の負荷で goroutine の起動が遅れても取りこぼさない値にしてある。
const cmdWait = 100 * time.Millisecond

// keyScan は打鍵に対する Cmd の束から取り出した値。
type keyScan struct {
	chrome    page.ChromeMsg
	hasChrome bool
	global    page.GlobalKeyMsg
	hasGlobal bool
}

// scanKey は打鍵の結果の束を辿り、最初の ChromeMsg と page が差し戻した
// page.GlobalKeyMsg を 1 回の走査で取り出す。
//
// **束の形は仮定しない。** page が差し戻すときの束は tea.Batch(自分の結果, BubbleKey)
// だが、tea.Batch は Cmd が 1 本になると束を畳むため、入れ子になるかどうかは page が
// 一覧の Cmd を持つかで変わる。実 page は持つので入れ子になり、pagetest.Spy は持たない
// ので tea.Batch(chrome, BubbleKey) と平らになる。「先頭が ChromeMsg なら差し戻しは
// 無い」といった形への依存は、平らな束で差し戻しを黙って取りこぼす。
//
// 代わりに時間で見分ける。**すぐに値を返さない Cmd は探しているものではない。**
// 各 Cmd を別 goroutine で実行して cmdWait で打ち切れば、点滅を待たずに済み、束の形にも
// 依存しない。打ち切りの代償を払うのは「差し戻しが無く、かつ点滅の Cmd がある」場合
// （＝絞り込みの入力中に閉じ込められた打鍵）だけである。
func scanKey(cmd tea.Cmd) keyScan {
	var got keyScan
	scanInto(cmd, &got)
	return got
}

// scanInto は Cmd を実行し、束ならその中身を辿って got を埋める。
func scanInto(cmd tea.Cmd, got *keyScan) {
	if cmd == nil || got.hasGlobal {
		return
	}
	msg, ok := runCmd(cmd)
	if !ok {
		return
	}
	if inner, ok := asCmds(msg); ok {
		for _, c := range inner {
			scanInto(c, got)
		}
		return
	}
	switch m := msg.(type) {
	case page.ChromeMsg:
		if !got.hasChrome {
			got.chrome, got.hasChrome = m, true
		}
	case page.GlobalKeyMsg:
		got.global, got.hasGlobal = m, true
	}
}

// runCmd は Cmd を別 goroutine で実行し、cmdWait を過ぎたら諦める。
//
// 諦めた goroutine は点滅の Tick が満了したときに終わる。チャネルに緩衝を持たせて
// あるので、受け取り手がもう居なくても送信で詰まって残ることはない。
func runCmd(cmd tea.Cmd) (tea.Msg, bool) {
	done := make(chan tea.Msg, 1)
	go func() { done <- cmd() }()

	select {
	case msg := <-done:
		return msg, true
	case <-time.After(cmdWait):
		return nil, false
	}
}

// findBubbled は page が差し戻したグローバルキーを返す。
func findBubbled(cmd tea.Cmd) (page.GlobalKeyMsg, bool) {
	got := scanKey(cmd)
	return got.global, got.hasGlobal
}

// statusLine は親が描く状態行を返す。
//
// 組み立ては chrome の純粋関数が持ち、親は値を写すだけである（chrome.go）。
func statusLine(a App) string { return chrome.Status(a.chromeView()) }
