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
	if _, global, ok := scanKey(cmd); ok {
		return update(a, global)
	}
	return a, cmd
}

// scanKey は打鍵の結果の束を辿り、最初の ChromeMsg と page が差し戻したキーを返す。
//
// **束の中の Cmd はすべて実行する**（pagetest.Msgs）。「差し戻しは無い」という結論は
// 形でも時間でも安全には出せない。
//
// 形では判らない。tea.Batch は Cmd が 1 本になると束を畳むため、絞り込み入力中の
// 閉じ込め tea.Batch(chrome, カーソル点滅) と、一覧が Cmd を返さない通常の打鍵の
// 差し戻し tea.Batch(chrome, BubbleKey) が同じ形（先頭が ChromeMsg の 2 要素）になる。
// 「先頭が ChromeMsg なら差し戻しは無い」は成り立たない（モーダル表示中は一覧の Cmd が
// nil で束ごと畳まれ、ChromeMsg 単体になる）。
//
// 時間でも判らない。遅い Cmd を打ち切ると、予算を超えて遅れた差し戻しが「閉じ込め
// られた」と同じ結果になる。press1 はそのとき nil を返し、gate_test の 6 つの
// assertion は nil で満たされるので、**取りこぼしが静かな成功に化ける**（実際に化ける
// には BubbleKey の単純なクロージャが予算を超える必要があり観測はされていないが、
// 失敗の向きが「黙って緑」である以上 Issue #31 の主題に反する）。
//
// **走査する Cmd はすべて有限時間で返ることが前提。** 長寿命の購読を Cmd で返す page
// （Logs タブなど）を親のテストに載せるときは、この走査を通さないこと。
//
// 代償は絞り込み入力中の打鍵ごとに点滅（約 0.5 秒）を待つことで、
// TestFilterInputConfinesGlobalKeysBeforeChromeArrives が約 2 秒かかる。**意図して
// 受け入れている**（回帰ガードが取りこぼしを成功と区別できないより遅いほうがまし）。
// 形と遅延の網羅は TestScanKeyFindsBubbleInEveryShape が固定している。
func scanKey(cmd tea.Cmd) (page.ChromeMsg, page.GlobalKeyMsg, bool) {
	var (
		chrome    page.ChromeMsg
		hasChrome bool
		global    page.GlobalKeyMsg
		hasGlobal bool
	)
	for _, msg := range pagetest.Msgs(cmd) {
		switch m := msg.(type) {
		case page.ChromeMsg:
			if !hasChrome {
				chrome, hasChrome = m, true
			}
		case page.GlobalKeyMsg:
			global, hasGlobal = m, true
		}
	}
	return chrome, global, hasGlobal
}

// statusLine は親が描く状態行を返す。
//
// 組み立ては chrome の純粋関数が持ち、親は値を写すだけである（chrome.go）。
func statusLine(a App) string { return chrome.Status(a.chromeView()) }
