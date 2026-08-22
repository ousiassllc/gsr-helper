package action

import (
	"testing"

	"charm.land/bubbles/v2/key"

	"github.com/ousiassllc/gsr-helper/internal/ui/page"
)

// 操作の識別（Issue #34）を検証する。決定が ID のまま表示層を往復すること、
// 同じ先頭キーを持つ 2 つの操作が検出されること、表を組むのが描画ごとでないこと。

// 選択肢には ID の識別子が載り、そのまま解き直せる。
//
// キー文字列で往復させると、キーを差し替えたときに決定が黙って別の操作へ移る。
func TestChoicesCarryActionID(t *testing.T) {
	set := testActions()
	items := set.Choices(sampleRunner(), fullCaps())
	acts := set.List()
	if len(items) != len(acts) {
		t.Fatalf("選択肢の件数 = %d, want %d", len(items), len(acts))
	}

	for i, item := range items {
		if item.ID == "" {
			t.Fatalf("%d 番目（%s）の識別子が空である", i, item.Desc)
		}
		got, ok := Of(item.ID)
		if !ok {
			t.Fatalf("識別子 %q が解けない", item.ID)
		}
		if got != acts[i].ID {
			t.Errorf("%d 番目の識別子 = %v, want %v", i, got, acts[i].ID)
		}
		// キーは「どのキーで選ばれたか」を伝えるだけで、同一性には使わない。
		if item.Key != acts[i].Key {
			t.Errorf("%d 番目のキー = %q, want %q", i, item.Key, acts[i].Key)
		}
	}
}

// 表示層が付けた解けない識別子は Unknown として弾く。
func TestActionOfRejectsUnknown(t *testing.T) {
	for _, id := range []string{"", "でたらめ", Unknown.String()} {
		if got, ok := Of(id); ok || got != Unknown {
			t.Errorf("Of(%q) = %v/%v, want Unknown/false", id, got, ok)
		}
	}
}

// 2 つの操作が同じ先頭キーを持つと、表を組む時点で検出される。
//
// 黙って上書きすると片方の操作が判定表のどの行にも当たらなくなり、理由が
// page.ReasonUnsupported にすり替わる（Issue #34）。
func TestNewActionSetRejectsDuplicateKey(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("同じ先頭キーを持つ操作が検出されていない")
		}
	}()

	keys := testKeys().Runner
	// 削除（D）を停止（x）と同じキーに割り当てる。
	keys.Delete = key.NewBinding(
		key.WithKeys(page.BindingKey(keys.Stop)),
		key.WithHelp("x", "削除"),
	)
	NewSet(keys)
}

// 表はキー定義から 1 度だけ組み、描画のたびには組み直さない。
//
// 以前は可否を 1 件求めるたびに 11 要素の map を確保しており、フッタ 1 行の描画で
// 9 回作っていた（Issue #34）。
func TestActionSetIsBuiltOnce(t *testing.T) {
	keys := testKeys().Runner
	set := NewSet(keys)
	r, caps := sampleRunner(), fullCaps()

	// 組み済みの表を引くだけの判定は 1 度も確保しない。
	//
	// 相対比較（組み済み < 組み直し）では退行を検出できない。組み直しの確保回数は
	// 定義上「組み済みの確保回数 + NewSet の確保回数」であり、Allowed が内部で表を
	// 組み直すようになっても不等号は成り立ったままだからである。
	if got := testing.AllocsPerRun(100, func() { set.Allowed("x", r, caps) }); got != 0 {
		t.Errorf("組み済みの表での確保 = %.0f 回, want 0 回（表が判定のたびに組み直されている）", got)
	}

	// 表を組む側は確保する。0 回の主張が「そもそも何も確保しない処理」を見ている
	// だけではないことを確かめる。
	if got := testing.AllocsPerRun(100, func() { NewSet(keys) }); got == 0 {
		t.Error("表の組み立てで 1 度も確保していない（前提が崩れている）")
	}
}
