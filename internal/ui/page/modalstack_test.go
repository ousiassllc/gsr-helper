package page

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/key"

	"github.com/ousiassllc/gsr-helper/internal/ui/keymap"
)

// モーダルの重なりが種類の数に依らないこと（後続 Issue が Register だけで足せること）を
// 固定するテストを集める。

// 種類を登録すれば、重なりの規則（キーは最上位だけ・esc は 1 枚）はそのまま効く。
//
// 後続 Issue が Confirm / DiffApproval / DrainWaiter を足すときに、この共有ファイルの
// 分岐を増やさずに済むことを固定する（Overlay の doc）。
func TestOverlayRegisteredModalFollowsStackRules(t *testing.T) {
	const kindConfirm ModalKind = "confirm"

	o := newOverlay()
	o.Register(kindConfirm, newStub("確認しますか"))

	o.Open(kindFirst, nil)
	o.Open(kindConfirm, nil)
	o, _ = sendOverlay(o, "j", "y")

	stub := stubOf(t, o, kindConfirm)
	if len(stub.keys) != 2 {
		t.Errorf("登録したモーダルが受け取ったキー = %v, want 2 件", stub.keys)
	}
	if got := stubOf(t, o, kindFirst).keys; len(got) != 0 {
		t.Errorf("背後のモーダルが受け取ったキー = %v, want 0 件（キーが背後に流れている）", got)
	}
	if got := o.Hints(); len(got) != 1 || got[0].Key != "y" {
		t.Errorf("フッタのヒント = %+v, want 登録したモーダルのもの", got)
	}
	if !strings.Contains(o.View(), "確認しますか") {
		t.Error("登録したモーダルの中身が描かれていない")
	}

	// esc は 1 枚だけ閉じ、背後のモーダルが最上位に戻る。
	o, _ = sendOverlay(o, "esc")
	if !strings.Contains(o.View(), "1 枚目") {
		t.Error("esc で 2 枚とも閉じている")
	}
}

// ? に出すキーの範囲は画面ごとに差し替えられる。
//
// 差し替えられないと、Set にキーの種類が増えるたびに全画面のヘルプへ他のタブの
// キーが並ぶ（keymap.Set.Help の doc）。
func TestOverlayHelpScopeIsPerPage(t *testing.T) {
	drain := testKeys().Runner.Drain.Help().Desc

	// 幅と高さを広く取る。bubbles/help はグループを列に並べて幅で落とすため、
	// 狭い領域では範囲の違いではなく列落ちを見てしまう。
	o := NewOverlay(testTab, testKeys(), testStyles(), true)
	o.SetState(state(200, 40))
	o.OpenHelp()
	if !strings.Contains(o.View(), drain) {
		t.Fatalf("既定のヘルプに runner の操作キー（%s）が無い", drain)
	}

	// 一覧のキーだけを持つ画面に差し替える。
	o.SetHelpScope(func(s keymap.Set) [][]key.Binding {
		return s.Help(s.List.Bindings())
	})
	if strings.Contains(o.View(), drain) {
		t.Errorf("差し替えたヘルプに範囲外のキー（%s）が出ている:\n%s", drain, o.View())
	}
	if !strings.Contains(o.View(), testKeys().List.Filter.Help().Desc) {
		t.Error("差し替えたヘルプに自分の範囲のキーが無い")
	}
}
