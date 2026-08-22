package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/exec"
	"github.com/ousiassllc/gsr-helper/internal/runner"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/pagetest"
)

// press1 は打鍵を 1 つ送り、page が返した ChromeMsg と、差し戻しを親が解釈した結果の
// Cmd を返す。**キーが page に閉じ込められたときの Cmd は nil である。** page 自身の
// Cmd は返さない。絞り込み中はそこにカーソル点滅の Cmd（約 0.5 秒ブロックする）が
// 混じっており、呼び出し側が isQuit などで実行すると 1 打鍵ごとにその時間だけ
// 待たされる。
// 閉じ込めが外れたかどうかは、親が差し戻しを解釈した結果の Cmd に終了などが現れる
// かで判定する。
//
// **打鍵は page → 親の往復を経る**（helper_test の sendKey と同じ）。App.Update を
// 1 回呼ぶだけの update では page.GlobalKeyMsg が親へ戻らず、閉じ込めを判定する経路
// そのものが走らない。update で書いていた頃は runners.handleKey の閉じ込めを丸ごと
// 消してもこのファイルの 2 つのテストが緑のままで、回帰ガードとして何も守って
// いなかった（Issue #31）。
//
// ChromeMsg は取り出すだけで**親へは渡さない**。このファイルの前提は「親が持つ
// モーダル・入力の状態は 1 打鍵ぶん古い」であり、渡すと検証したい経路が消える。
// 取り出すのは閉じ込めの前提（モーダルが開いた・絞り込みが始まった）を page 側の
// 値で確かめるためである。
func press1(a App, k string) (App, page.ChromeMsg, tea.Cmd) {
	next, cmd := update(a, press(k))

	c, _ := firstChrome(cmd)

	// 差し戻しは束の最上位に置かれる（page は tea.Batch(自分の結果, BubbleKey) を返す）。
	// 束を走査するときに page 自身の Cmd を実行しないことは findBubbled が担う
	// （helper_test の doc）。
	global, ok := findBubbled(cmd)
	if !ok {
		return next, c, nil
	}
	next, cmd = update(next, global)
	return next, c, cmd
}

// モーダルを開いた直後の打鍵でも、グローバルキーは背後へ抜けない。
//
// **ChromeMsg を親へ渡さずに検証する。** 親が持つモーダル・入力の状態は page からの
// Msg で更新されるため、素早い連続打鍵では 1 打鍵ぶん古い。以前は親がその古い値で
// 配送を判断しており、確認中に打った q でアプリが終わり、1 でタブが変わりえた
// （page.GlobalKeyMsg の doc）。閉じ込めるのはモーダルを持つ page 自身である。
func TestModalConfinesGlobalKeysBeforeChromeArrives(t *testing.T) {
	a := newApp(exec.NewFake())
	a, _ = update(a, tea.WindowSizeMsg{Width: 100, Height: 30})
	a, _ = update(a, discoveredMsg{
		seq:    1,
		result: runner.Result{Runners: []runner.Runner{pagetest.SampleRunner()}},
		err:    nil,
	})

	// enter で詳細のモーダルが開く。親へ ChromeMsg は渡さない（1 打鍵ぶん古い状態）。
	a, c, _ := press1(a, "enter")
	if a.chrome.Modal {
		t.Fatal("ChromeMsg を渡していないのに親がモーダルを認識している（前提が崩れている）")
	}
	if !c.Modal {
		t.Fatal("enter でモーダルが開いていない（前提が崩れている）")
	}

	// 直後の q で終了しない。
	a, _, cmd := press1(a, "q")
	if isQuit(cmd) {
		t.Error("モーダル表示中の q でアプリが終了した")
	}

	// 直後の番号キーでタブも変わらない。
	next, _, _ := press1(a, "2")
	if next.active != 0 {
		t.Errorf("モーダル表示中の番号キーでタブが %d に変わった", next.active)
	}

	// 直後の r でも検出は走らない（モーダル表示中はキーがモーダルに閉じ込められる）。
	if after, _, _ := press1(a, "r"); after.inflight != 0 {
		t.Errorf("モーダル表示中の r で検出が走った（inflight = %d）", after.inflight)
	}
}

// 絞り込みの入力中も同じ規則が働く。runner 名が数字を含むため、1〜7 を機能キーの
// まま残すと名前で絞り込めない（screens.md の入力中）。
func TestFilterInputConfinesGlobalKeysBeforeChromeArrives(t *testing.T) {
	a := newApp(exec.NewFake())
	a, _ = update(a, tea.WindowSizeMsg{Width: 100, Height: 30})
	a, _ = update(a, discoveredMsg{
		seq:    1,
		result: runner.Result{Runners: []runner.Runner{pagetest.SampleRunner()}},
		err:    nil,
	})

	a, c, _ := press1(a, "/")
	if a.chrome.Input != "" {
		t.Fatal("ChromeMsg を渡していないのに親が入力中を認識している（前提が崩れている）")
	}
	if c.Input == "" {
		t.Fatal("/ で絞り込みが始まっていない（前提が崩れている）")
	}

	a, _, cmd := press1(a, "q")
	if isQuit(cmd) {
		t.Error("入力中の q でアプリが終了した")
	}
	// 1 は選択中のタブ自身なので、誤って親へ抜けても active は変わらない。
	// 抜けたことが分かるよう別のタブの番号を打つ。
	next, _, _ := press1(a, "2")
	if next.active != 0 {
		t.Errorf("入力中の番号キーでタブが %d に変わった", next.active)
	}
	if after, _, _ := press1(a, "r"); after.inflight != 0 {
		t.Errorf("入力中の r で検出が走った（inflight = %d）", after.inflight)
	}
}

// 検出中の再読み込みは、黙って何もせず理由を状態行に出す（Issue #49）。
//
// 待たされる時間は最大 discoverBudget（15 秒）あり、無反応だと「効かないキー」に
// 見える（screens.md の設計原則 2）。無効なタブの番号キーは既に理由を出している。
func TestRefreshDuringDiscoveryShowsNotice(t *testing.T) {
	a := newApp(exec.NewFake())
	a, _ = update(a, tea.WindowSizeMsg{Width: 100, Height: 30})

	// 最初の Tick で検出が走り出す。
	a, _ = update(a, tickMsg{})
	if a.inflight == 0 {
		t.Fatal("検出が始まっていない（前提が崩れている）")
	}

	before := a.inflight
	a, cmd := sendKey(a, "r")
	if a.inflight != before {
		t.Errorf("検出中の r で検出が重なった（inflight = %d, want %d）", a.inflight, before)
	}
	if isQuit(cmd) {
		t.Fatal("r で終了している")
	}
	if !strings.Contains(statusLine(a), "検出中です") {
		t.Errorf("状態行 = %q, want 検出中である旨の案内", statusLine(a))
	}

	// 案内は次の打鍵で消える（状態行に残り続けない）。
	a, _ = sendKey(a, "j")
	if strings.Contains(statusLine(a), "検出中です") {
		t.Errorf("次の打鍵の後も案内が残っている（状態行 = %q）", statusLine(a))
	}
}
