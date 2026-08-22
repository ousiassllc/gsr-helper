package action

import (
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/ui/atom"
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
//
// 能力がすべて揃った systemd 管理の runner では、実装済みのサービス制御だけが有効に
// なり、残りは未対応の理由を持つ。有効なら理由は空、無効なら理由が要る（理由の無い
// グレーアウトは、利用者に打ち直しても無駄だと伝えられない）。
func TestHintsCarryReasons(t *testing.T) {
	// **期待値を被テスト関数の入力から作らない。** Hints は keys.Footer() を 1 件ずつ
	// 並べる実装なので、len(keys.Footer()) と比べると Footer が空になった日に両辺 0 で
	// 素通りし、下のループが 0 回で緑になる（塞いだつもりの穴がそのまま残る）。
	// screens.md のフッタが定める件数をリテラルで置く（TestHintsMatchSpecFooter と同じ）。
	const want = 9

	// フッタの 9 キーのうち実装済みは開始・停止・強制停止・ドレイン停止の 4 つ。
	// 削除・追加・更新・設定・ログはこの版では未対応である。
	enabled := map[string]bool{"s": true, "x": true, "X": true, "d": true}

	hints := testActions().Hints(sampleRunner(), fullCaps(), testKeys().Runner)
	if len(hints) != want {
		t.Fatalf("ヒントの件数 = %d, want %d", len(hints), want)
	}

	for _, h := range hints {
		if h.Enabled != enabled[h.Key] {
			t.Errorf("キー %q の可否 = %v, want %v", h.Key, h.Enabled, enabled[h.Key])
		}
		if h.Enabled == (h.Reason != "") {
			t.Errorf("キー %q = %v/%q, 可否と理由の有無が食い違う", h.Key, h.Enabled, h.Reason)
		}
	}
}
