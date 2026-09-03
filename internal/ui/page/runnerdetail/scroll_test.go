package runnerdetail

import (
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/ui/page/pagetest"
)

// 別の runner の詳細を開くと情報部のスクロールが先頭に戻る（Issue #30）。
//
// 80x24 では情報部に配れる行数が 1 行まで潰れる（Model.infoHeight）ため、位置を
// 持ち越すと**別の runner の詳細が読んでいた場所から始まり、前の runner の続きを
// 今の runner の情報として読むことになる**。既存のテストは高さ 20 / 24 / 40 の
// 本体領域を使っており情報部がスクロールしないため、この経路を通らなかった。
func TestOpenResetsInfoScroll(t *testing.T) {
	// 80x24 の端末で本体に残る領域に相当する大きさ。
	o := newOverlayWithDetail(80, 20)
	Open(&o, pagetest.SampleRunner(), pagetest.Caps())

	// 情報部だけをページ送りする（j / k は操作リストのカーソル移動）。
	o, _ = o.Update(pagetest.Press("ctrl+f"))
	if got := detailOf(t, o).info.Offset(); got == 0 {
		t.Fatalf("情報部がスクロールしていない（前提が崩れている。位置 = %d）", got)
	}

	// 別の runner を開く。
	other := pagetest.StandaloneRunner()
	other.Dir = "/opt/runners/build01-2"
	Open(&o, other, pagetest.Caps())

	if got := detailOf(t, o).info.Offset(); got != 0 {
		t.Errorf("別の runner を開いた後の情報部の位置 = %d, want 0", got)
	}
}

// 同じ対象の状態が変わっただけでは情報部のスクロールを戻さない。
//
// 3 秒ごとの再検出で先頭へ戻ると、読んでいた場所を失う（Model.SetState の doc）。
func TestStateKeepsInfoScroll(t *testing.T) {
	o := newOverlayWithDetail(80, 20)
	Open(&o, pagetest.SampleRunner(), pagetest.Caps())

	o, _ = o.Update(pagetest.Press("ctrl+f"))
	want := detailOf(t, o).info.Offset()
	if want == 0 {
		t.Fatal("情報部がスクロールしていない（前提が崩れている）")
	}

	o.SetState(pagetest.State(80, 20, pagetest.BusyRunner()))

	if got := detailOf(t, o).info.Offset(); got != want {
		t.Errorf("再検出の後の情報部の位置 = %d, want %d", got, want)
	}
}
