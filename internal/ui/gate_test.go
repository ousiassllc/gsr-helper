package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/exec"
	"github.com/ousiassllc/gsr-helper/internal/runner"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/pagetest"
)

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
	a, _ = update(a, press("enter"))
	if a.chrome.Modal {
		t.Fatal("ChromeMsg を渡していないのに親がモーダルを認識している（前提が崩れている）")
	}

	// 直後の q で終了しない。
	a, cmd := update(a, press("q"))
	if isQuit(cmd) {
		t.Error("モーダル表示中の q でアプリが終了した")
	}

	// 直後の番号キーでタブも変わらない。
	next, _ := update(a, press("2"))
	if next.active != 0 {
		t.Errorf("モーダル表示中の番号キーでタブが %d に変わった", next.active)
	}

	// 直後の r でも検出は走らない（モーダル表示中はキーがモーダルに閉じ込められる）。
	if after, _ := update(a, press("r")); after.inflight != 0 {
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

	a, _ = update(a, press("/"))
	if a.chrome.Input != "" {
		t.Fatal("ChromeMsg を渡していないのに親が入力中を認識している（前提が崩れている）")
	}

	a, cmd := update(a, press("q"))
	if isQuit(cmd) {
		t.Error("入力中の q でアプリが終了した")
	}
	next, _ := update(a, press("1"))
	if next.active != 0 {
		t.Errorf("入力中の番号キーでタブが %d に変わった", next.active)
	}
}
