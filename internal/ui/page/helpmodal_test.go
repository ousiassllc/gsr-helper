package page

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/key"

	"github.com/ousiassllc/gsr-helper/internal/ui/keymap"
)

// ヘルプのスクロール位置が自動更新（3 秒ごとの共有状態）をまたいで保持される。
//
// 修正前は、StateMsg を受けるたびに pane.NewHelp で作り直した直後に SetOffset を
// 呼んでおり、その時点では高さが 0 なので位置が 0 に丸められていた。続く SizeMsg で
// 高さが入っても位置は戻らないため、**ヘルプを読んでいる間ずっと先頭へ戻され続けた**
// （Issue #30）。
func TestHelpKeepsOffsetAcrossStateUpdates(t *testing.T) {
	// 高さを絞ってスクロールが要る状態にする。幅は広く取り、bubbles/help の列落ちで
	// 行数が変わることの影響を避ける。
	o, _ := NewOverlay(testTab, state(200, 12))
	o.OpenHelp()

	if !helpOf(t, o).help.Scrollable() {
		t.Fatal("ヘルプがスクロールできない（前提が崩れている）")
	}

	o, _ = sendOverlay(o, "j", "j", "j")
	want := helpOf(t, o).help.Offset()
	if want == 0 {
		t.Fatal("スクロールしても位置が 0 のままである（前提が崩れている）")
	}

	// 自動更新の 1 周期が届く（Overlay は StateMsg を送ってから SizeMsg を送る）。
	o.SetState(state(200, 12))

	if got := helpOf(t, o).help.Offset(); got != want {
		t.Errorf("自動更新の後の位置 = %d, want %d（先頭へ戻されている）", got, want)
	}
}

// SetHelpScope で範囲を差し替えても、読んでいたスクロール位置は保たれる。
//
// 登録し直さず Msg で伝えるのは位置を失わないためである（scopeMsg の doc）。範囲を
// 差し替える側も、組み直した Help へ**大きさを配り直してから**位置を戻さなければ
// ならない。順序を落とすと SetOffset は高さ 0 で丸められ、位置は 0 に潰れる
// （pane.Help.SetOffset の丸め。Issue #30）。
func TestHelpKeepsOffsetAcrossScopeChange(t *testing.T) {
	o, _ := NewOverlay(testTab, state(200, 12))
	o.OpenHelp()

	o, _ = sendOverlay(o, "j", "j", "j")
	want := helpOf(t, o).help.Offset()
	if want == 0 {
		t.Fatal("スクロールしても位置が 0 のままである（前提が崩れている）")
	}
	before := helpOf(t, o).help.View()

	// 既定と同じグループを並び順だけ変えて渡す。行数（＝上限）を変えずに中身だけを
	// 差し替えられるので、位置が動いたら丸め直しではなく取りこぼしである。
	o.SetHelpScope(func(s keymap.Set) [][]key.Binding {
		return s.Help(s.Runner.Order(), s.List.Bindings(), s.List.FilterBindings())
	})

	h := helpOf(t, o)
	if got := h.help.View(); got == before {
		t.Fatal("範囲が差し替わっていない（前提が崩れている）")
	}
	if got := h.help.Offset(); got != want {
		t.Errorf("範囲を差し替えた後の位置 = %d, want %d（先頭へ戻されている）", got, want)
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
	o, _ := NewOverlay(testTab, state(200, 40))
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

// helpOf は登録済みのヘルプのモーダルを取り出す。
func helpOf(t *testing.T, o Overlay) helpModal {
	t.Helper()

	m, ok := o.Modal(ModalHelp)
	if !ok {
		t.Fatal("ヘルプが登録されていない")
	}
	h, ok := m.Model.(helpModal)
	if !ok {
		t.Fatalf("ヘルプの Model = %T, want helpModal", m.Model)
	}
	return h
}
