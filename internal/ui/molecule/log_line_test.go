package molecule

import (
	"strings"
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/ui/token"
)

// 強調する行だけが装飾され、本文は失われない（FR-25）。
func TestLogLine(t *testing.T) {
	colored := token.NewStyles(true, true)
	const text = "[2026-08-21 12:05:44Z ERROR JobRunner] failed"

	plain := LogLine(text, token.RolePlain, colored)
	if plain != text {
		t.Errorf("強調しない行 = %q, want 装飾なしの本文", plain)
	}

	fail := LogLine(text, token.RoleFail, colored)
	warn := LogLine(text, token.RoleWarn, colored)
	if fail == plain || warn == plain {
		t.Errorf("強調されていない（fail=%q warn=%q）", fail, warn)
	}
	if fail == warn {
		t.Errorf("ERROR と WARN が同じ装飾になっている: %q", fail)
	}
	for _, got := range []string{fail, warn} {
		if !strings.Contains(got, text) {
			t.Errorf("装飾した行 = %q, want 本文 %q を含む", got, text)
		}
	}
}

// 色を無効にした設定では、どの役割でも本文だけを返す。
func TestLogLineWithoutColor(t *testing.T) {
	plainStyles := token.NewStyles(true, false)
	const text = "[WARN] x"

	for _, role := range []token.RoleToken{token.RolePlain, token.RoleWarn, token.RoleFail} {
		if got := LogLine(text, role, plainStyles); got != text {
			t.Errorf("色なしの行 = %q, want %q", got, text)
		}
	}
}
