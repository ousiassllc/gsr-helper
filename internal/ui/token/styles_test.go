package token

import (
	"testing"

	"charm.land/lipgloss/v2"
)

// fields はスタイルの全フィールドを名前付きで返す。
// フィールドを増やしたときにテストから漏れることを防ぐため、
// 件数の一致も併せて検証する。
func fields(s Styles) map[string]lipgloss.Style {
	return map[string]lipgloss.Style{
		"OK":          s.OK,
		"Warn":        s.Warn,
		"Fail":        s.Fail,
		"Skip":        s.Skip,
		"Muted":       s.Muted,
		"Accent":      s.Accent,
		"Danger":      s.Danger,
		"Header":      s.Header,
		"TabActive":   s.TabActive,
		"TabInactive": s.TabInactive,
		"TabDisabled": s.TabDisabled,
		"Cursor":      s.Cursor,
		"Selected":    s.Selected,
		"Divider":     s.Divider,
	}
}

const styleFieldCount = 14

func TestFieldsCoversAllStyles(t *testing.T) {
	if got := len(fields(NewStyles(true, true))); got != styleFieldCount {
		t.Fatalf("fields が返すフィールド数 = %d, want %d（Styles にフィールドを追加したらテストも追う）", got, styleFieldCount)
	}
}

// 色を使わない場合はすべて素通しになる。記号は残るため状態は判別できる。
func TestNewStylesWithoutColorIsTransparent(t *testing.T) {
	samples := []string{"● active", "x:停止", "", "日本語の文字列"}
	for _, dark := range []bool{true, false} {
		for name, st := range fields(NewStyles(dark, false)) {
			for _, s := range samples {
				if got := st.Render(s); got != s {
					t.Errorf("dark=%v %s.Render(%q) = %q, 素通しでなければならない", dark, name, s, got)
				}
			}
		}
	}
}

// 背景の明暗で配色が変わる。同じ色を使うと白背景で読めない状態が残る。
func TestNewStylesDiffersByBackground(t *testing.T) {
	darkStyles := fields(NewStyles(true, true))
	lightStyles := fields(NewStyles(false, true))
	for name, d := range darkStyles {
		l := lightStyles[name]
		if d.Render("x") == l.Render("x") {
			t.Errorf("%s の配色が明背景と暗背景で同じである", name)
		}
	}
}

func TestNewStylesWithColorRenders(t *testing.T) {
	for name, st := range fields(NewStyles(true, true)) {
		if got := st.Render("x"); got == "x" {
			t.Errorf("%s に色が付いていない", name)
		}
	}
}

// 色を付ける役割には色が付き、RolePlain と未定義の役割は素通しになる。
func TestStyleByRole(t *testing.T) {
	s := NewStyles(true, true)
	for _, role := range allRoles() {
		got := s.Style(role).Render("x")
		if role == RolePlain {
			if got != "x" {
				t.Errorf("Style(RolePlain) = %q, 素通しでなければならない", got)
			}
			continue
		}
		if got == "x" {
			t.Errorf("Style(%d) に色が付いていない", role)
		}
	}
	// 未定義の役割は素通しのスタイルを返す（panic させない）。
	if got := s.Style(RoleToken(99)).Render("x"); got != "x" {
		t.Errorf("Style(未定義) = %q, 素通しでなければならない", got)
	}
}

// 色が無効なときは Style も素通しになる。
func TestStyleWithoutColor(t *testing.T) {
	s := NewStyles(true, false)
	for _, role := range allRoles() {
		if got := s.Style(role).Render("x"); got != "x" {
			t.Errorf("Style(%d) = %q, 素通しでなければならない", role, got)
		}
	}
}
