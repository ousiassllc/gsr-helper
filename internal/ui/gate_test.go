package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/exec"
	"github.com/ousiassllc/gsr-helper/internal/runner"
	"github.com/ousiassllc/gsr-helper/internal/ui/discovery"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/pagetest"
)

// press1 は打鍵を 1 つ送り、page が返した ChromeMsg と、差し戻しを親が解釈した
// 結果の Cmd を返す。**閉じ込められたときの Cmd は nil である**（page 自身の Cmd は
// 返さない。絞り込み中は点滅の Cmd が混じり、isQuit で実行すると待たされる）。
//
// **打鍵は page → 親の往復を経る**（helper_test の sendKey と同じ）。App.Update を
// 1 回呼ぶだけの update では page.GlobalKeyMsg が親へ戻らず、閉じ込めの判定が走らない。
// update で書いていた頃は runners.handleKey の閉じ込めを消しても緑のままだった
// （Issue #31）。ChromeMsg は取り出すだけで**親へは渡さない**（前提は「親の状態は
// 1 打鍵ぶん古い」であり、渡すと検証したい経路が消える）。「親が page より先に
// キーを解釈する」退行は app_keys_test の spy 経由の検証が受け持つ。
func press1(a App, k string) (App, page.ChromeMsg, tea.Cmd) {
	next, cmd := update(a, press(k))

	// 閉じ込められた打鍵では nil を返す（親は打鍵を見ていないので Cmd も無い）。
	// 差し戻しを取りこぼして nil になる経路は無い（pagetest.ScanKey の doc）。取りこぼしが
	// nil に化けると 6 つの assertion がすべて満たされて静かに緑になる。
	c, global, ok := pagetest.ScanKey(cmd)
	if !ok {
		return next, c, nil
	}
	next, cmd = update(next, global)
	return next, c, cmd
}

// press1 の陽性対照。閉じ込めの無い状態では、差し戻しが親へ届いて解釈される。
//
// **これが無いと下の 2 つのテストが空振りに戻る。** press1 を「往復せず update を
// 1 回呼ぶだけ」に戻すと差し戻しは親へ届かなくなるが、下の 2 つは「親が反応しない」
// ことを見ているので緑のままになる（Issue #31 の元の退行そのもの）。往復が生きて
// いることをここで固定しておけば、その変異はこのテストが落として知らせる。
func TestPress1DeliversBubbledKeyWhenNotConfined(t *testing.T) {
	a := newApp(exec.NewFake())
	a, _ = update(a, tea.WindowSizeMsg{Width: 100, Height: 30})
	a, _ = update(a, discovery.Msg{
		Seq:    1,
		Result: runner.Result{Runners: []runner.Runner{pagetest.SampleRunner()}},
		Err:    nil,
	})

	if _, _, cmd := press1(a, "q"); !isQuit(cmd) {
		t.Error("閉じ込めの無い状態で q が親へ届いていない")
	}
	if next, _, _ := press1(a, "2"); next.active != 1 {
		t.Errorf("閉じ込めの無い状態で 2 が親へ届いていない（active = %d）", next.active)
	}
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
	a, _ = update(a, discovery.Msg{
		Seq:    1,
		Result: runner.Result{Runners: []runner.Runner{pagetest.SampleRunner()}},
		Err:    nil,
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
	a, _ = update(a, discovery.Msg{
		Seq:    1,
		Result: runner.Result{Runners: []runner.Runner{pagetest.SampleRunner()}},
		Err:    nil,
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
// 待たされる時間は最大 discovery.Budget（15 秒）あり、無反応だと「効かないキー」に
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
