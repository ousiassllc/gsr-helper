package runnerdetail

import (
	"strings"
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/ui/page"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/pagetest"
)

// testTab は検証で使うタブ番号。0 以外にして、決定に載る番号が既定値でないことを見る。
const testTab = 1

// newOverlayWithDetail は詳細画面を登録した重なりを返す（タブが New でするのと同じ）。
func newOverlayWithDetail(w, h int) page.Overlay {
	o := page.NewOverlay(testTab, pagetest.Keys(), pagetest.Styles(), true)
	o.Register(Kind, New(pagetest.State(w, h)))
	o.SetSize(w, h)
	return o
}

// detailOf は登録済みの詳細画面を取り出す。
//
// page.Overlay は中身を tea.Model としてしか持たない（page.Modal の doc）ため、
// テストから状態を見るには取り出す必要がある。
func detailOf(t *testing.T, o page.Overlay) Model {
	t.Helper()

	m, ok := o.Modal(Kind)
	if !ok {
		t.Fatal("詳細が登録されていない")
	}
	d, ok := m.Model.(modal)
	if !ok {
		t.Fatalf("詳細の Model = %T, want modal", m.Model)
	}
	return d.detail
}

// 登録した詳細画面は重なりの規則に従い、キーは最上位のときだけ届く。
func TestDetailInOverlayReceivesKeysWhenTopmost(t *testing.T) {
	o := newOverlayWithDetail(80, 20)
	_ = Open(&o, pagetest.SampleRunner(), pagetest.Caps())
	if !o.Active() {
		t.Fatal("詳細を開いたのに Active が偽である")
	}

	o, _ = o.Update(pagetest.Press("j"))
	o, _ = o.Update(pagetest.Press("j"))
	if got := detailOf(t, o).Cursor(); got != 2 {
		t.Fatalf("詳細のカーソル = %d, want 2", got)
	}

	// ヘルプを重ねると、同じキーは最上位にのみ渡る。
	_ = o.OpenHelp()
	o, _ = o.Update(pagetest.Press("j"))
	if got := detailOf(t, o).Cursor(); got != 2 {
		t.Errorf("背後の詳細のカーソル = %d, want 2（キーが背後に流れている）", got)
	}

	// esc は 1 枚だけ閉じ、詳細が最上位に戻る。
	o, _ = o.Update(pagetest.Press("esc"))
	if !strings.Contains(o.View(), "build01-1") {
		t.Error("esc で 2 枚とも閉じている")
	}
}

// 開いている詳細は共有状態で作り直され、カーソルは保たれる。
//
// 開いた時点のスナップショットを持ち続けると、3 秒ごとの再検出で状態が変わっても
// 画面は古いままで、**ジョブを取り始めた runner に「削除できる」と提示してしまう**
// （Model.SetState の doc）。
func TestDetailRefreshesFromState(t *testing.T) {
	keys := pagetest.Keys()
	_, busyReason := page.Allowed("D", pagetest.BusyRunner(), pagetest.Caps(), keys.Runner)
	if busyReason == "" {
		t.Fatal("ジョブ実行中の削除が塞がれていない（前提が崩れている）")
	}

	o := newOverlayWithDetail(80, 20)
	_ = Open(&o, pagetest.SampleRunner(), pagetest.Caps())
	o, _ = o.Update(pagetest.Press("j"))
	if got := detailOf(t, o).Cursor(); got != 1 {
		t.Fatalf("詳細のカーソル = %d, want 1", got)
	}
	if strings.Contains(o.View(), busyReason) {
		t.Fatal("ジョブを取る前から実行中の理由が出ている")
	}

	// 同じ runner（同じ Dir）がジョブを取り始めた周期が届く。
	o.SetState(pagetest.State(80, 20, pagetest.BusyRunner()))

	if !o.Active() {
		t.Error("共有状態の反映でモーダルが閉じている")
	}
	if got := detailOf(t, o).Cursor(); got != 1 {
		t.Errorf("反映後のカーソル = %d, want 1（操作を選んでいる途中で動かさない）", got)
	}
	if !strings.Contains(o.View(), busyReason) {
		t.Errorf("ジョブ実行中の可否が反映されていない:\n%s", o.View())
	}

	// 検出結果に対象が無い周期では今の値を保つ（一時的な検出漏れで空にしない）。
	o.SetState(pagetest.State(80, 20))
	if !strings.Contains(o.View(), "build01-1") {
		t.Error("検出漏れの周期で詳細が空になっている")
	}
}
