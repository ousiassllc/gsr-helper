package report_test

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/ousiassllc/gsr-helper/internal/ui/page/setup/report"
)

// 幅を超える行は折り返す。切り詰めると失敗の理由と対処が末尾から消える。
func TestWrapFoldsLongLinesInsteadOfTruncating(t *testing.T) {
	t.Parallel()

	const width = 20
	const hint = "  → 権限が不足しています。必要なスコープは admin:org です"

	got := report.Wrap([]string{hint}, width)
	if len(got) < 2 {
		t.Fatalf("折り返していない: %q", got)
	}
	for _, l := range got {
		if w := lipgloss.Width(l); w > width {
			t.Errorf("行 %q の表示幅が %d（上限 %d）", l, w, width)
		}
	}
	// 中身は失われない（切り詰めではないこと）。
	if joined := strings.Join(got, ""); joined != hint {
		t.Errorf("連結 = %q, want %q", joined, hint)
	}
}

// 幅に収まる行はそのまま返す。
func TestWrapKeepsShortLines(t *testing.T) {
	t.Parallel()

	in := []string{"完了: 2 台", "未実行: 1 台（build01-4）"}
	got := report.Wrap(in, 80)
	if len(got) != len(in) || got[0] != in[0] || got[1] != in[1] {
		t.Errorf("Wrap = %q, want %q", got, in)
	}
}

// 幅が 1 未満のときは何もしない。最初のリサイズが届く前は BodyW が 0 である。
func TestWrapDoesNothingWithoutWidth(t *testing.T) {
	t.Parallel()

	in := []string{"完了: 2 台"}
	if got := report.Wrap(in, 0); len(got) != 1 || got[0] != in[0] {
		t.Errorf("Wrap = %q, want %q", got, in)
	}
}
