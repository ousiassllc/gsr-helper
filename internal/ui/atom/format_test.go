package atom

import (
	"math"
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

func TestBytes(t *testing.T) {
	cases := []struct {
		n    int64
		want string
	}{
		{0, "0B"},
		{512, "512B"},
		{1023, "1023B"},
		{1024, "1.0K"},
		{1024*1024 - 1, "1.0M"}, // 丸めると 1024.0K になる値は 1 段上げる
		{104858, "102.4K"},
		{25877477785, "24.1G"},
		{107374182, "102.4M"},
		{1024 * 1024 * 1024 * 1024, "1.0T"},
		{math.MaxInt64, "8388608.0T"}, // T より上の単位は置かない
		{-1, token.IconNoUnit},
		{-1024, token.IconNoUnit},
	}
	for _, c := range cases {
		if got := Bytes(c.n); got != c.want {
			t.Errorf("Bytes(%d) = %q, want %q", c.n, got, c.want)
		}
	}
}

func TestFiles(t *testing.T) {
	cases := []struct {
		n    int64
		want string
	}{
		{0, "0"},
		{1, "1"},
		{999, "999"},
		{1000, "1,000"},
		{12004, "12,004"},
		{412003, "412,003"},
		{1000000, "1,000,000"},
		{math.MaxInt64, "9,223,372,036,854,775,807"},
		{-1, token.IconNoUnit},
	}
	for _, c := range cases {
		if got := Files(c.n); got != c.want {
			t.Errorf("Files(%d) = %q, want %q", c.n, got, c.want)
		}
	}
}

func TestRatio(t *testing.T) {
	cases := []struct {
		name     string
		pct      int
		warn     bool
		want     string
		wantRole token.RoleToken
	}{
		{"閾値内", 82, false, "82%", token.RolePlain},
		{"閾値超過", 91, true, "91% " + token.IconWarn, token.RoleWarn},
		{"下限", 0, false, "0%", token.RolePlain},
		{"上限", 100, false, "100%", token.RolePlain},
		{"負の値は 0 に丸める", -5, false, "0%", token.RolePlain},
		{"100 を超える値は丸める", 101, true, "100% " + token.IconWarn, token.RoleWarn},
	}
	for _, c := range cases {
		got, role := Ratio(c.pct, c.warn)
		if got != c.want {
			t.Errorf("%s: Ratio(%d, %v) = %q, want %q", c.name, c.pct, c.warn, got, c.want)
		}
		if role != c.wantRole {
			t.Errorf("%s: Ratio(%d, %v) の役割 = %d, want %d", c.name, c.pct, c.warn, role, c.wantRole)
		}
	}
}
