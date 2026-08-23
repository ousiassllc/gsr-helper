package atom

import (
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/ousiassllc/gsr-helper/internal/ui/token"
)

func TestDuration(t *testing.T) {
	cases := []struct {
		d    time.Duration
		want string
	}{
		{0, "0s"},
		{59 * time.Second, "59s"},
		{time.Minute, "1m00s"},
		{4*time.Minute + 12*time.Second, "4m12s"},
		{22*time.Minute + 3*time.Second, "22m03s"},
		{time.Hour + 2*time.Minute, "1h02m"},
		{100*time.Hour + 30*time.Minute, "100h30m"},
		{-1 * time.Second, token.IconNoUnit},
		{-100 * time.Hour, token.IconNoUnit},
	}
	for _, c := range cases {
		if got := Duration(c.d); got != c.want {
			t.Errorf("Duration(%v) = %q, want %q", c.d, got, c.want)
		}
	}
}

func TestVersionText(t *testing.T) {
	cases := []struct {
		name     string
		cur      string
		latest   string
		want     string
		wantRole token.RoleToken
	}{
		{"同値なら注意記号なし", "2.311.0", "2.311.0", "2.311.0", token.RolePlain},
		{"差異があれば注意記号", "2.309.0", "2.311.0", "2.309.0 " + token.IconWarn, token.RoleWarn},
		{"最新が未取得なら注意記号なし", "2.309.0", "", "2.309.0", token.RolePlain},
		{"現行が空", "", "2.311.0", token.IconNoUnit, token.RoleMuted},
		{"長いバージョン", "2.311.0-beta.1", "2.311.0", "2.311.0-beta.1 " + token.IconWarn, token.RoleWarn},
	}
	for _, c := range cases {
		got, role := VersionText(c.cur, c.latest)
		if got != c.want {
			t.Errorf("%s: VersionText(%q, %q) = %q, want %q", c.name, c.cur, c.latest, got, c.want)
		}
		if role != c.wantRole {
			t.Errorf("%s: VersionText(%q, %q) の役割 = %d, want %d", c.name, c.cur, c.latest, role, c.wantRole)
		}
	}
}

// パスの末尾だけを残すときも書記素を割らない（Issue #50）。
//
// 先頭を残す余裕が無い幅では Path が中略記号 + 末尾側だけを返す。以前は rune を
// 末尾から数えており、ZWJ 絵文字や結合文字を含むパスでは結合の途中で切れて、
// 中略記号の直後に単独の ZWJ や結合文字が並ぶ壊れた列が出た。
func TestPathTailKeepsGraphemes(t *testing.T) {
	const family = "\U0001F468\u200d\U0001F469\u200d\U0001F467" // ZWJ で結合する絵文字
	const accented = "e\u0301e\u0301"                           // 結合文字（アクセント）付きの 2 文字

	// 末尾が書記素で終わるパスを使う。末尾に ASCII が付いていると、切る位置が
	// 書記素に届かず検証にならない。
	tests := map[string]string{
		"ZWJ 絵文字": "/opt/runners/" + family,
		"結合文字":    "/opt/runners/" + accented,
	}

	for name, p := range tests {
		t.Run(name, func(t *testing.T) {
			for w := 1; w <= 10; w++ {
				got := Path(p, w)
				if lipgloss.Width(got) > w {
					t.Errorf("幅 %d の結果 = %q（幅 %d）, want 幅 %d 以下",
						w, got, lipgloss.Width(got), w)
				}
				// 中略記号の直後が結合の途中（単独の ZWJ / 結合文字）になっていない。
				rest, cut := strings.CutPrefix(got, token.IconEllipsis)
				if !cut {
					continue
				}
				for _, r := range []rune{'\u200d', '\u0301'} {
					if strings.HasPrefix(rest, string(r)) {
						t.Errorf("幅 %d の結果 = %q, want 書記素の途中で切らない", w, got)
					}
				}
			}
		})
	}
}

// サイズは 1024 を基数に、SIZE 列の幅に収まる桁で表記する。
func TestBytes(t *testing.T) {
	cases := map[string]struct {
		n    int64
		want string
	}{
		"0":         {0, "0B"},
		"1023 まではB": {1023, "1023B"},
		"1024 でK":   {1024, "1.0K"},
		"端数は 1 桁":   {1536, "1.5K"},
		"M":         {107374182, "102.4M"},
		"G":         {2 << 30, "2.0G"},
		"負は記号":      {-1, token.IconNoUnit},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			if got := Bytes(c.n); got != c.want {
				t.Errorf("Bytes(%d) = %q, want %q", c.n, got, c.want)
			}
		})
	}
}

// どの桁でも SIZE 列の幅（7）に収まる。収まらないと一覧の桁が溢れる。
func TestBytesFitsColumnWidth(t *testing.T) {
	for _, n := range []int64{0, 1023, 1024, 1<<20 - 1, 1 << 40, 1 << 50, 1<<63 - 1} {
		if got := Bytes(n); len(got) > token.SizeColumnWidth {
			t.Errorf("Bytes(%d) = %q（%d 桁）, want %d 桁以内", n, got, len(got), token.SizeColumnWidth)
		}
	}
}
