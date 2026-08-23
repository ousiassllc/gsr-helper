package atom

import (
	"strings"
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/ui/token"
)

// 判定は記号と文字の対で返す。色を使えない端末でも 4 つを判別できるようにするため。
func TestDoctorStatus(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		in       token.StateToken
		wantText string
		wantRole token.RoleToken
	}{
		"OK":   {in: token.StateOK, wantText: "✓ OK", wantRole: token.RoleOK},
		"WARN": {in: token.StateWarn, wantText: "⚠ WARN", wantRole: token.RoleWarn},
		"FAIL": {in: token.StateFail, wantText: "✗ FAIL", wantRole: token.RoleFail},
		"SKIP": {in: token.StateSkip, wantText: "⊘ SKIP", wantRole: token.RoleSkip},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			text, role := DoctorStatus(tt.in)
			if text != tt.wantText {
				t.Errorf("text = %q, want %q", text, tt.wantText)
			}
			if role != tt.wantRole {
				t.Errorf("role = %v, want %v", role, tt.wantRole)
			}
		})
	}
}

// 4 つの表記はすべて違う。同じになると色の無い端末で判別できない。
func TestDoctorStatusTextsAreDistinct(t *testing.T) {
	t.Parallel()

	seen := make(map[string]bool)
	for _, st := range []token.StateToken{token.StateOK, token.StateWarn, token.StateFail, token.StateSkip} {
		text, _ := DoctorStatus(st)
		if seen[text] {
			t.Errorf("表記 %q が重複している", text)
		}
		seen[text] = true
	}
}

// 未定義の判定でも空文字を返さない。空だと列が詰まって桁がずれる。
func TestDoctorStatusUnknown(t *testing.T) {
	t.Parallel()

	text, role := DoctorStatus(token.StateToken(99))
	if strings.TrimSpace(text) == "" {
		t.Error("未定義の判定で空文字を返した")
	}
	if role != token.RoleMuted {
		t.Errorf("role = %v, want %v", role, token.RoleMuted)
	}
}

// STATUS 列の幅（token.DoctorColumns の 7）に最長の表記が収まる。
func TestDoctorStatusFitsColumnWidth(t *testing.T) {
	t.Parallel()

	var width int
	for _, c := range token.DoctorColumns() {
		if c.ID == token.ColStatus {
			width = c.Width
		}
	}
	if width == 0 {
		t.Fatal("STATUS 列が DoctorColumns に無い")
	}
	for _, st := range []token.StateToken{token.StateOK, token.StateWarn, token.StateFail, token.StateSkip} {
		text, _ := DoctorStatus(st)
		if got := len([]rune(text)); got > width {
			t.Errorf("%q は %d 文字で、列幅 %d に収まらない", text, got, width)
		}
	}
}
