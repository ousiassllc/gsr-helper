package atom

import (
	"regexp"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/ousiassllc/gsr-helper/internal/ui/token"
)

func TestCellHasExactWidth(t *testing.T) {
	cases := []struct {
		name  string
		s     string
		width int
	}{
		{"ASCII が収まる", "build01-1", 20},
		{"ASCII がちょうど", "build01-1", 9},
		{"ASCII が溢れる", "build01-longname-1", 9},
		{"全角が収まる", "日本語ランナー", 20},
		{"全角がちょうど", "日本語", 6},
		{"全角が溢れる", "日本語ランナー", 6},
		{"全角の境界で 1 セル余る", "日本語ランナー", 5},
		{"混在", "runner-日本語-01", 12},
		{"空文字", "", 8},
		{"幅 1", "日本語", 1},
	}
	for _, c := range cases {
		for _, a := range []Align{Left, Right} {
			got := Cell(c.s, c.width, a)
			if w := lipgloss.Width(got); w != c.width {
				t.Errorf("%s: Cell(%q, %d, %v) の幅 = %d, want %d（%q）", c.name, c.s, c.width, a, w, c.width, got)
			}
		}
	}
}

func TestCellAlign(t *testing.T) {
	if got, want := Cell("ab", 5, Left), "ab   "; got != want {
		t.Errorf("Cell 左寄せ = %q, want %q", got, want)
	}
	if got, want := Cell("ab", 5, Right), "   ab"; got != want {
		t.Errorf("Cell 右寄せ = %q, want %q", got, want)
	}
}

func TestCellWithNonPositiveWidth(t *testing.T) {
	for _, w := range []int{0, -1, -100} {
		if got := Cell("build01-1", w, Left); got != "" {
			t.Errorf("Cell(_, %d, _) = %q, want 空文字", w, got)
		}
	}
}

func TestTruncate(t *testing.T) {
	cases := []struct {
		s     string
		width int
		want  string
	}{
		{"abcde", 5, "abcde"},  // ちょうど
		{"abcdef", 5, "abcd…"}, // 1 超過
		{"abcdefgh", 5, "abcd…"},
		{"abc", 5, "abc"},
		{"", 5, ""},
		{"abcde", 1, "…"},
		{"a", 1, "a"},
		{"日本語", 6, "日本語"},
		{"日本語です", 6, "日本…"},
		{"日本語です", 5, "日本…"}, // 全角の境界で 1 セル余る
		{"abcde", 0, ""},
		{"abcde", -1, ""},
	}
	for _, c := range cases {
		if got := Truncate(c.s, c.width); got != c.want {
			t.Errorf("Truncate(%q, %d) = %q, want %q", c.s, c.width, got, c.want)
		}
	}
}

// Pad は幅を超えても切り詰めないので、装飾済みの文字列にも使える。
//
// 入力は colorStyles で組む。plainStyles では Render が恒等（色が無効）になり、
// ANSI 列を 1 つも含まない文字列を相手に「装飾済みでも使える」ことを検証した
// つもりになる（Issue #47）。前提が崩れたら気付けるよう、入力に ANSI 列が
// 含まれること自体をここで確かめる。
func TestPadDoesNotTruncate(t *testing.T) {
	styled := colorStyles().OK.Render("● active")
	if !strings.ContainsRune(styled, '\x1b') {
		t.Fatalf("入力が装飾されていない: %q（colorStyles が色を出していない）", styled)
	}

	// 表示幅（8 セル）より狭い幅を渡しても、装飾ごとそのまま返る。
	if got := Pad(styled, 3, Left); got != styled {
		t.Errorf("Pad は幅を超えても切り詰めない: %q", got)
	}
	// 埋める幅は ANSI 列を除いた表示幅から数える。
	if got, want := Pad(styled, 12, Left), styled+"    "; got != want {
		t.Errorf("装飾済みの Pad = %q, want %q", got, want)
	}
	if got, want := Pad("ab", 4, Right), "  ab"; got != want {
		t.Errorf("Pad 右寄せ = %q, want %q", got, want)
	}
}

func TestPath(t *testing.T) {
	const p = "/opt/runners/build01-1/_work/bar"
	cases := []struct {
		name  string
		p     string
		width int
		want  string
	}{
		{"収まる", p, 40, p},
		{"ちょうど", p, lipgloss.Width(p), p}, // 幅はバイト数ではなく表示セル数
		{"中間を中略する", p, 24, "/opt/…/_work/bar"},
		{"末尾だけ残す", p, 8, "…ork/bar"},
		{"空文字", "", 10, ""},
		{"幅 0", p, 0, ""},
		{"幅が負", p, -3, ""},
		{"相対パス", "runners/build01-1/_work/bar", 20, "runners/…/_work/bar"},
	}
	for _, c := range cases {
		got := Path(c.p, c.width)
		if got != c.want {
			t.Errorf("%s: Path(%q, %d) = %q, want %q", c.name, c.p, c.width, got, c.want)
		}
		if c.width > 0 && lipgloss.Width(got) > c.width {
			t.Errorf("%s: Path(%q, %d) の幅 %d が上限を超えている", c.name, c.p, c.width, lipgloss.Width(got))
		}
	}
}

func TestDivider(t *testing.T) {
	s := plainStyles()

	plain := Divider(10, "", s)
	if plain != strings.Repeat(token.IconDivider, 10) {
		t.Errorf("見出し無しの区切り線 = %q", plain)
	}

	titled := Divider(20, "孤児ユニット", s)
	if !strings.HasPrefix(titled, token.IconDivider+" 孤児ユニット ") {
		t.Errorf("見出し付きの区切り線 = %q", titled)
	}
	if w := lipgloss.Width(titled); w != 20 {
		t.Errorf("見出し付きの区切り線の幅 = %d, want 20", w)
	}

	if got := Divider(0, "孤児ユニット", s); got != "" {
		t.Errorf("幅 0 の区切り線 = %q, want 空文字", got)
	}
	if w := lipgloss.Width(Divider(5, "とても長い見出し", s)); w > 5 {
		t.Errorf("幅に収まらない見出しで幅 %d になった", w)
	}
}

func TestJustify(t *testing.T) {
	if got, want := Justify("s:開始", "", 30), "s:開始"; got != want {
		t.Errorf("右側が空の Justify = %q, want %q", got, want)
	}

	got := Justify("s:開始", "（稼働中のため不要）", 40)
	if w := lipgloss.Width(got); w != 40 {
		t.Errorf("Justify の幅 = %d, want 40（%q）", w, got)
	}

	// 幅が足りなくても右側（理由）を落とさない。
	narrow := Justify("s:開始", "（稼働中のため不要）", 4)
	if !strings.Contains(narrow, "（稼働中のため不要）") {
		t.Errorf("幅不足で理由が落ちた: %q", narrow)
	}
}

func TestJoin(t *testing.T) {
	parts := []string{"s:開始", "x:停止", "X:強制停止"}

	all := Join(parts, " ", 40, "?:ヘルプ")
	for _, p := range parts {
		if !strings.Contains(all, p) {
			t.Errorf("収まる幅で %q が落ちた: %q", p, all)
		}
	}

	narrow := Join(parts, " ", 20, "?:ヘルプ")
	if !strings.Contains(narrow, "?:ヘルプ") {
		t.Errorf("溢れたときに overflow が付かない: %q", narrow)
	}
	if lipgloss.Width(narrow) > 20 {
		t.Errorf("Join の幅 %d が上限を超えた: %q", lipgloss.Width(narrow), narrow)
	}

	if got := Join(parts, " ", 0, "?:ヘルプ"); got != "" {
		t.Errorf("幅 0 の Join = %q, want 空文字", got)
	}
	if got := Join(parts, " ", 3, "?:ヘルプ"); lipgloss.Width(got) > 3 {
		t.Errorf("overflow も収まらない幅で %q（幅 %d）", got, lipgloss.Width(got))
	}
	if got := Join(parts, " ", 20, ""); strings.Contains(got, "X:強制停止") {
		t.Errorf("overflow が空のときは落とすだけ: %q", got)
	}
}

// parts がちょうど収まる幅では overflow の分を予算から引かない。
//
// 無条件に先取りすると、収まっているのに全ての parts が落ちて overflow だけが残る。
func TestJoinKeepsPartsThatExactlyFit(t *testing.T) {
	parts := []string{"s:開始", "x:停止"}
	const want = "s:開始 x:停止"
	width := lipgloss.Width(want)

	if got := Join(parts, " ", width, "?:ヘルプ"); got != want {
		t.Errorf("ちょうど収まる幅 %d の Join = %q, want %q", width, got, want)
	}
	// 1 セル足りなければ落として overflow を付ける。
	got := Join(parts, " ", width-1, "?:ヘルプ")
	if !strings.Contains(got, "?:ヘルプ") {
		t.Errorf("1 セル足りない幅の Join = %q, overflow が付かない", got)
	}
	if w := lipgloss.Width(got); w > width-1 {
		t.Errorf("Join の幅 = %d, 上限 %d を超えた（%q）", w, width-1, got)
	}
}

// 装飾済みの文字列を切っても ANSI 列を割らない。
//
// chromebar.KeyBar は装飾済みのキーヒントを atom.Join に渡し、Join は幅が足りない
// 分を Truncate で中略する。CSI の途中で切ると端末が後続の出力を飲み込む。
// 期待値を端末の色数に左右されないよう、エスケープを直接組んで渡す。
func TestTruncateKeepsANSISequenceIntact(t *testing.T) {
	const styled = "\x1b[31mbuild01-longname-1\x1b[0m"
	csi := regexp.MustCompile("\x1b\\[[0-9;]*m")

	for _, width := range []int{1, 2, 3, 5, 9, 17} {
		got := Truncate(styled, width)
		if w := lipgloss.Width(got); w > width {
			t.Errorf("Truncate(装飾済み, %d) の幅 = %d, want %d 以下（%q）", width, w, width, got)
		}
		// 完全な CSI 列を取り除いてもエスケープ文字が残るなら、途中で切れている。
		if rest := csi.ReplaceAllString(got, ""); strings.ContainsRune(rest, '\x1b') {
			t.Errorf("Truncate(装飾済み, %d) が ANSI 列を割った: %q", width, got)
		}
		if want := token.IconEllipsis; !strings.HasSuffix(got, want) {
			t.Errorf("Truncate(装飾済み, %d) = %q, want %q で終わる", width, got, want)
		}
	}

	// 見える文字は装飾しない場合と同じだけ残す。
	if got, want := csi.ReplaceAllString(Truncate(styled, 5), ""), Truncate("build01-longname-1", 5); got != want {
		t.Errorf("装飾を除いた結果 = %q, want %q", got, want)
	}
}
