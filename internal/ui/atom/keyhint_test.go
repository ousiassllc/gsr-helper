package atom

import (
	"strings"
	"testing"
)

func TestKeyHintEnabled(t *testing.T) {
	got := KeyHint(Hint{Key: "x", Desc: "停止", Enabled: true}, plainStyles())
	if want := "x:停止"; got != want {
		t.Errorf("KeyHint(有効) = %q, want %q", got, want)
	}
}

// 無効でもキーを消さない。理由は Hint に保持したまま KeyBar がまとめて出す。
func TestKeyHintDisabledKeepsKey(t *testing.T) {
	h := Hint{Key: "x", Desc: "停止", Enabled: false, Reason: "root 権限が必要です"}
	got := KeyHint(h, plainStyles())
	if !strings.Contains(got, "x:停止") {
		t.Errorf("KeyHint(無効) = %q, キーと説明を消してはならない", got)
	}
	if h.Reason != "root 権限が必要です" {
		t.Errorf("KeyHint が Hint の理由を書き換えた: %q", h.Reason)
	}
}

// 色を使えない端末でも有効・無効を区別できる（screens.md の設計原則 4）。
//
// グレーアウトだけでは NO_COLOR 相当の環境で 1 文字も変わらないため、
// 丸括弧という色以外の手がかりを添える。
func TestKeyHintDisabledDiffersWithoutColor(t *testing.T) {
	enabled := Hint{Key: "x", Desc: "停止", Enabled: true}
	disabled := Hint{Key: "x", Desc: "停止", Enabled: false, Reason: "root 権限が必要です"}

	for name, s := range map[string]bool{"色なし": false, "色あり": true} {
		styles := plainStyles()
		if s {
			styles = colorStyles()
		}
		on, off := KeyHint(enabled, styles), KeyHint(disabled, styles)
		if on == off {
			t.Errorf("%s: 有効なキーと無効なキーの表示が同じである（%q）", name, on)
		}
	}

	if got, want := KeyHint(disabled, plainStyles()), "(x:停止)"; got != want {
		t.Errorf("KeyHint(無効) = %q, want %q", got, want)
	}
}

func TestKeyHintDegenerateInput(t *testing.T) {
	s := plainStyles()
	cases := []struct {
		name string
		h    Hint
		want string
	}{
		{"空の Hint", Hint{}, ""},
		{"キーのみ", Hint{Key: "?", Enabled: true}, "?"},
		{"説明のみ", Hint{Desc: "ヘルプ", Enabled: true}, "ヘルプ"},
		{"無効でキーのみ", Hint{Key: "?", Reason: "理由"}, "(?)"},
	}
	for _, c := range cases {
		if got := KeyHint(c.h, s); got != c.want {
			t.Errorf("%s: KeyHint = %q, want %q", c.name, got, c.want)
		}
	}
	// 色が有効でも空入力で余計な装飾が残らない。
	if got := KeyHint(Hint{}, colorStyles()); strings.Contains(got, ":") {
		t.Errorf("空の Hint に区切りが付いた: %q", got)
	}
}
