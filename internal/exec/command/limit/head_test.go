package limit

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestHeadMarksOnlyWhenCutting(t *testing.T) {
	// Tail と向きが逆で、残すのは末尾側・印は前に付く。
	tests := []struct {
		name string
		s    string
		n    int
		want string
	}{
		{"上限より短い", "abc", 8, "abc"},
		{"上限と同じ", "abcdefgh", 8, "abcdefgh"},
		{"上限を 1 バイト超える", "abcdefghi", 8, Prefix + "bcdefghi"},
		{"上限が 0", "abc", 0, Prefix},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Head(tt.s, tt.n); got != tt.want {
				t.Errorf("Head(%q, %d) = %q, want %q", tt.s, tt.n, got, tt.want)
			}
		})
	}

	// 切っていないのに印が付くと、読み手は前に続きがあると誤解する。
	if got := Head("だめでした: 権限がありません", sampleLimit); strings.Contains(got, Prefix) {
		t.Errorf("上限以下なのに省略の印が付いている: %q", got)
	}
}

func TestHeadNeverLeavesBrokenUTF8(t *testing.T) {
	// バイト数で切ると多バイト文字の途中で切れる。Head は末尾側を残すので断片は
	// **先頭**に残り、落とさないと Prefix の直後が壊れた文字になる
	// （dropPartialRuneAtStart）。この関数が存在する理由そのものの経路である。
	const runes = 8
	s := strings.Repeat(multiByte, runes)
	size := len(multiByte)

	for n := range len(s) + 1 {
		got := Head(s, n)
		if !utf8.ValidString(got) {
			t.Errorf("Head(_, %d) が壊れた UTF-8 を残した: %q", n, got)
			continue
		}
		if strings.ContainsRune(got, utf8.RuneError) {
			t.Errorf("Head(_, %d) に置換文字が混じった: %q", n, got)
		}
		if n >= len(s) {
			if got != s {
				t.Errorf("Head(_, %d) = %q, want 素通り", n, got)
			}
			continue
		}
		// 切った場合は「上限に収まる最大の文字数」だけが末尾側から残る。断片を
		// 落とす処理が無いと、印の直後が 1〜2 バイトの断片になる。
		want := Prefix + strings.Repeat(multiByte, n/size)
		if got != want {
			t.Errorf("Head(_, %d) = %q, want %q", n, got, want)
		}
	}
}
