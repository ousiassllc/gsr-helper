package page

import (
	"testing"
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
	o := NewOverlay(testTab, testKeys(), testStyles(), true)
	o.SetState(state(200, 12))
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
