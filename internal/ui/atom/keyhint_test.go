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

// 無効なキーでも表示幅を増やさない。
//
// 丸括弧で囲むと 1 つあたり 2 セル増え、幅 80 のフッタ 1 行目に screens.md が定める
// 9 個のキーが収まらなくなる。色を使えない端末で有効・無効を読み分ける手がかりは
// フッタ 2 行目（molecule.KeyBar）と操作リストの理由（molecule.ActionRow）が担う。
func TestKeyHintDisabledKeepsWidth(t *testing.T) {
	enabled := Hint{Key: "x", Desc: "停止", Enabled: true}
	disabled := Hint{Key: "x", Desc: "停止", Enabled: false, Reason: "root 権限が必要です"}

	if got, want := KeyHint(disabled, plainStyles()), "x:停止"; got != want {
		t.Errorf("KeyHint(無効) = %q, want %q", got, want)
	}
	if on, off := KeyHint(enabled, plainStyles()), KeyHint(disabled, plainStyles()); on != off {
		t.Errorf("無効なキーの表示幅が有効なキーと違う（%q / %q）", on, off)
	}

	// 色が使える端末ではグレーアウトで区別できる。
	on, off := KeyHint(enabled, colorStyles()), KeyHint(disabled, colorStyles())
	if on == off {
		t.Errorf("色ありで有効なキーと無効なキーの表示が同じである（%q）", on)
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
		{"無効でキーのみ", Hint{Key: "?", Reason: "理由"}, "?"},
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
