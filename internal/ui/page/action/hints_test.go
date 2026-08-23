package action

import (
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/ui/atom"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
)

// フッタのキーヒント（Set.Hints）の検証を集める。可否の判定そのものは allow_test.go。

// フッタ 1 行目は screens.md の共通レイアウトの並びと短い表記そのものである。
//
// 幅 80 に 9 個 + ?:ヘルプ が収まる表記でなければ設計原則 1 を満たせないため、
// 文言も含めて仕様側に固定する。
func TestHintsMatchSpecFooter(t *testing.T) {
	want := []atom.Hint{
		{Key: "s", Desc: "開始"}, {Key: "x", Desc: "停止"}, {Key: "X", Desc: "強制"},
		{Key: "d", Desc: "ドレイン"}, {Key: "D", Desc: "削除"}, {Key: "n", Desc: "追加"},
		{Key: "u", Desc: "更新"}, {Key: "e", Desc: "設定"}, {Key: "l", Desc: "ログ"},
	}

	hints := testActions().Hints(sampleRunner(), fullCaps(), testKeys().Runner)
	if len(hints) != len(want) {
		t.Fatalf("ヒントの件数 = %d, want %d", len(hints), len(want))
	}
	for i, w := range want {
		if hints[i].Key != w.Key || hints[i].Desc != w.Desc {
			t.Errorf("%d 番目のヒント = %q/%q, want %q/%q",
				i, hints[i].Key, hints[i].Desc, w.Key, w.Desc)
		}
	}
}

// フッタのキーヒントは可否と理由を持つ。
//
// 件数を先に固定する。長さを見ずに回すと、Hints が空を返した日に「1 件も違反が
// 無かった」として通ってしまう（Issue #31）。
func TestHintsCarryReasons(t *testing.T) {
	// **期待値を被テスト関数の入力から作らない。** Hints は keys.Footer() を 1 件ずつ
	// 並べる実装なので、len(keys.Footer()) と比べると Footer が空になった日に両辺 0 で
	// 素通りし、下のループが 0 回で緑になる（塞いだつもりの穴がそのまま残る）。
	// screens.md のフッタが定める件数をリテラルで置く（TestHintsMatchSpecFooter と同じ）。
	const want = 9

	hints := testActions().Hints(sampleRunner(), fullCaps(), testKeys().Runner)
	if len(hints) != want {
		t.Fatalf("ヒントの件数 = %d, want %d", len(hints), want)
	}

	// 実装済みの操作は有効で理由を持たず、未実装の操作は無効で理由を持つ。
	// どちらか一方だけを見ると、全件が無効／全件が有効になった日に気づけない。
	enabled := map[string]bool{page.BindingKey(testKeys().Runner.Logs): true}
	for _, h := range hints {
		if h.Enabled != enabled[h.Key] {
			t.Errorf("キー %q の可否 = %v, want %v", h.Key, h.Enabled, enabled[h.Key])
		}
		if (h.Reason == "") != h.Enabled {
			t.Errorf("キー %q の可否 = %v なのに理由 = %q", h.Key, h.Enabled, h.Reason)
		}
	}
}
