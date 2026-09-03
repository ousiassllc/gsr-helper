package limit

import (
	"strings"
	"testing"
	"unicode/utf8"
)

const (
	// multiByte は 1 文字 3 バイトの文字。上限をバイト数で切ると必ず文字の途中に
	// 当たるので、断片を落とす経路（dropPartialRune*）を通る。
	multiByte = "あ"
	// sampleLimit は「上限を下回る入力は素通りする」ことだけを見る行で使う上限。
	// 実際の上限の値は呼び出し側（command の limits.go）の方針である。
	sampleLimit = 4 << 10
)

func TestTruncateTailMarksOnlyWhenCutting(t *testing.T) {
	// 印を必ず残すのは、短いメッセージと「切られた長いメッセージ」を読み手が
	// 区別できるようにするためである（Suffix の理由）。
	tests := []struct {
		name string
		s    string
		n    int
		want string
	}{
		{"上限より短い", "abc", 8, "abc"},
		{"上限と同じ", "abcdefgh", 8, "abcdefgh"},
		{"上限を 1 バイト超える", "abcdefghi", 8, "abcdefgh" + Suffix},
		{"上限が 0", "abc", 0, Suffix},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Tail(tt.s, tt.n); got != tt.want {
				t.Errorf("Tail(%q, %d) = %q, want %q", tt.s, tt.n, got, tt.want)
			}
		})
	}

	// 切っていないのに印が付くと、読み手は続きがあると誤解する。
	if got := Tail("だめでした: 権限がありません", sampleLimit); strings.Contains(got, Suffix) {
		t.Errorf("上限以下なのに省略の印が付いている: %q", got)
	}
}

func TestTruncateTailNeverLeavesBrokenUTF8(t *testing.T) {
	// バイト数で切ると多バイト文字の途中で切れる。壊れた文字を JSON に載せないため、
	// 末尾に残った不完全な断片は落としてから印を付ける（dropPartialRuneAtEnd）。
	const runes = 8
	s := strings.Repeat(multiByte, runes)
	size := len(multiByte)

	for n := range len(s) + 1 {
		got := Tail(s, n)
		if !utf8.ValidString(got) {
			t.Errorf("Tail(_, %d) が壊れた UTF-8 を残した: %q", n, got)
			continue
		}
		if strings.ContainsRune(got, utf8.RuneError) {
			t.Errorf("Tail(_, %d) に置換文字が混じった: %q", n, got)
		}
		if n >= len(s) {
			if got != s {
				t.Errorf("Tail(_, %d) = %q, want 素通り", n, got)
			}
			continue
		}
		// 切った場合は「上限に収まる最大の文字数」だけが残る。断片を落とす処理が
		// 無いと、ここが 1〜2 バイト多い壊れた文字列になる。
		want := strings.Repeat(multiByte, n/size) + Suffix
		if got != want {
			t.Errorf("Tail(_, %d) = %q, want %q", n, got, want)
		}
	}
}
