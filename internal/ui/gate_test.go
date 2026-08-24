package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/exec"
	"github.com/ousiassllc/gsr-helper/internal/ui/discovery"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
)

// 打鍵は page → 親の往復を経る（pagetest.Press1）。App.Update を 1 回呼ぶだけの
// update では page.GlobalKeyMsg が親へ戻らず、閉じ込めの判定が走らない。update で
// 書いていた頃は runners.handleKey の閉じ込めを消しても緑のままだった（Issue #31）。

// press1 の陽性対照。閉じ込めの無い状態では、差し戻しが親へ届いて解釈される。
//
// **これが無いと下のテストが空振りに戻る。** press1 を「往復せず update を 1 回呼ぶ
// だけ」に戻すと差し戻しは親へ届かなくなるが、下は「親が反応しない」ことを見ている
// ので緑のままになる（Issue #31 の元の退行そのもの）。往復が生きていることをここで
// 固定しておけば、その変異はこのテストが落として知らせる。
func TestPress1DeliversBubbledKeyWhenNotConfined(t *testing.T) {
	a := newAppWithRunner(exec.NewFake())

	if _, _, cmd := press1(a, "q"); !isQuit(t, cmd) {
		t.Error("閉じ込めの無い状態で q が親へ届いていない")
	}
	if next, _, _ := press1(a, "2"); next.active != 1 {
		t.Errorf("閉じ込めの無い状態で 2 が親へ届いていない（active = %d）", next.active)
	}
}

// モーダルを開いた直後・絞り込みを始めた直後の打鍵でも、グローバルキーは背後へ
// 抜けない。
//
// **ChromeMsg を親へ渡さずに検証する。** 親が持つモーダル・入力の状態は page からの
// Msg で更新されるため、素早い連続打鍵では 1 打鍵ぶん古い。以前は親がその古い値で
// 配送を判断しており、確認中に打った q でアプリが終わり、1 でタブが変わりえた
// （page.GlobalKeyMsg の doc）。閉じ込めるのはモーダルを持つ page 自身である。
//
// 絞り込みの入力中も同じ規則が働く。runner 名が数字を含むため、1〜7 を機能キーの
// まま残すと名前で絞り込めない（screens.md の入力中）。
func TestConfinesGlobalKeysBeforeChromeArrives(t *testing.T) {
	tests := map[string]struct {
		open  string                    // 閉じ込めを始める打鍵
		began func(page.ChromeMsg) bool // page 側がその状態になったか
		saw   func(App) bool            // 親がその状態を知ってしまっているか
	}{
		"モーダル表示中": {
			open:  "enter",
			began: func(c page.ChromeMsg) bool { return c.Modal },
			saw:   func(a App) bool { return a.chrome.Modal },
		},
		"入力中": {
			open:  "/",
			began: func(c page.ChromeMsg) bool { return c.Input != "" },
			saw:   func(a App) bool { return a.chrome.Input != "" },
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			a, c, _ := press1(newAppWithRunner(exec.NewFake()), tt.open)
			if tt.saw(a) {
				t.Fatalf("ChromeMsg を渡していないのに親が%sを認識している（前提が崩れている）", name)
			}
			if !tt.began(c) {
				t.Fatalf("%q を押しても%sになっていない（前提が崩れている）", tt.open, name)
			}

			// 直後の q で終了しない。
			if _, _, cmd := press1(a, "q"); isQuit(t, cmd) {
				t.Errorf("%sの q でアプリが終了した", name)
			}
			// 直後の番号キーでタブも変わらない。1 は選択中のタブ自身なので、誤って
			// 親へ抜けても active は変わらない。抜けたことが分かる別の番号を打つ。
			if next, _, _ := press1(a, "2"); next.active != 0 {
				t.Errorf("%sの番号キーでタブが %d に変わった", name, next.active)
			}
			// 直後の r でも検出は走らない（キーがモーダル・入力に閉じ込められる）。
			if after, _, _ := press1(a, "r"); after.disc.Seq() != 0 {
				t.Errorf("%sの r で検出が走った（seq = %d）", name, after.disc.Seq())
			}
		})
	}
}

// 検出中の再読み込みは、黙って何もせず理由を状態行に出す（Issue #49）。
//
// 待たされる時間は最大 discovery.Budget（15 秒）あり、無反応だと「効かないキー」に
// 見える（screens.md の設計原則 2）。無効なタブの番号キーは既に理由を出している。
//
// **手動の再読み込みも実行中の検出に重ねないことをここで併せて見る**（案内を出す前提
// そのものなので、通し番号が進まないことを案内の検証と同じ筋で固定しておく）。
func TestRefreshDuringDiscoveryShowsNotice(t *testing.T) {
	a := newApp(exec.NewFake())
	a, _ = update(a, tea.WindowSizeMsg{Width: 100, Height: 30})

	// 最初の Tick で検出が走り出す。
	a, _ = update(a, discovery.TickMsg{})
	if !a.disc.Busy() {
		t.Fatal("検出が始まっていない（前提が崩れている）")
	}

	before := a.disc.Seq()
	a, cmd := sendKey(a, "r")
	if a.disc.Seq() != before {
		t.Errorf("検出中の r で検出が重なった（seq = %d, want %d）", a.disc.Seq(), before)
	}
	if isQuit(t, cmd) {
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
