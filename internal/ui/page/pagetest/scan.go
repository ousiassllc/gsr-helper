package pagetest

import (
	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/ui/page"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/pagetest/cmdtest"
)

// ScanKey は打鍵の結果の束を辿り、最初の ChromeMsg と page が差し戻したキーを返す。
//
// 親 Model の検証は「page がキーを閉じ込めたか、親へ差し戻したか」で合否が決まるため、
// この判定を取りこぼすと閉じ込めが壊れていてもテストが緑になる。
//
// **束の中の Cmd はすべて実行する**（Msgs）。「差し戻しは無い」という結論は形でも
// 時間でも安全には出せない。
//
// 形では判らない。tea.Batch は Cmd が 1 本になると束を畳むため、絞り込み入力中の
// 閉じ込め tea.Batch(chrome, カーソル点滅) と、一覧が Cmd を返さない通常の打鍵の
// 差し戻し tea.Batch(chrome, BubbleKey) が同じ形（先頭が ChromeMsg の 2 要素）になる。
// 「先頭が ChromeMsg なら差し戻しは無い」は成り立たない（モーダル表示中は一覧の Cmd が
// nil で束ごと畳まれ、ChromeMsg 単体になる）。
//
// 時間でも判らない。遅い Cmd を打ち切ると、予算を超えて遅れた差し戻しが「閉じ込め
// られた」と同じ結果になり、**取りこぼしが静かな成功に化ける**（実際に化けるには
// BubbleKey の単純なクロージャが予算を超える必要があり観測はされていないが、失敗の
// 向きが「黙って緑」である以上 Issue #31 の主題に反する）。
//
// **走査する Cmd はすべて有限時間で返ることが前提。** 長寿命の購読を Cmd で返す page
// （Logs タブなど）を親のテストに載せるときは、この走査を通さないこと。前提が破れたら
// cmdtest.CmdTimeout で諦めて panic で止める（cmdtest.MustMsgs）——素で走らせていたころは
// そこで止まり、壊れ方が失敗ではなくハングになった（Issue #150）。
//
// 代償は絞り込み入力中の打鍵ごとに一覧のカーソル点滅（約 0.5 秒）を待つことである。
// **意図して受け入れている**（回帰ガードが取りこぼしを成功と区別できないより遅いほうが
// まし）。代償を消すなら走査ではなく点滅の側を触ること（organism/table の BlinkSpeed を
// 注入可能にする）。形と遅延の網羅は TestScanKeyFindsBubbleInEveryShape が固定している。
func ScanKey(cmd tea.Cmd) (page.ChromeMsg, page.GlobalKeyMsg, bool) {
	var (
		chrome    page.ChromeMsg
		hasChrome bool
		global    page.GlobalKeyMsg
		hasGlobal bool
	)
	for _, msg := range cmdtest.MustMsgs(cmd, cmdtest.CmdTimeout) {
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
