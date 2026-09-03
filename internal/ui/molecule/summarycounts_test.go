package molecule

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/ousiassllc/gsr-helper/internal/ui/token"
)

// 件数は 4 つとも出す。0 を省くと「無い」のか「数えていない」のか区別できない。
func TestSummaryCountsShowsEveryStatus(t *testing.T) {
	t.Parallel()

	got := SummaryCounts(SummaryView{OK: 12, Warn: 3, Fail: 0, Skip: 1}, 80, plainStyles())
	for _, want := range []string{"OK 12", "WARN 3", "FAIL 0", "SKIP 1"} {
		if !strings.Contains(got, want) {
			t.Errorf("%q が無い: %q", want, got)
		}
	}
}

// 幅に収まらない場合は中略する（他の molecule と同じ約束）。
func TestSummaryCountsTruncates(t *testing.T) {
	t.Parallel()

	got := SummaryCounts(SummaryView{OK: 12, Warn: 3, Fail: 3, Skip: 1}, 12, plainStyles())
	if w := lipgloss.Width(got); w > 12 {
		t.Errorf("幅 = %d, want 12 以下（%q）", w, got)
	}
	if !strings.Contains(got, token.IconEllipsis) {
		t.Errorf("中略記号が無い: %q", got)
	}
}

// 見出し行は件数と最終実行時刻を左右に振り分ける。
func TestSummaryLine(t *testing.T) {
	t.Parallel()

	got := SummaryLine(SummaryView{OK: 1}, "12:06:20", 60, plainStyles())
	if !strings.Contains(got, "OK 1") {
		t.Errorf("件数が無い: %q", got)
	}
	if !strings.Contains(got, "最終実行: 12:06:20") {
		t.Errorf("最終実行時刻が無い: %q", got)
	}
	if w := lipgloss.Width(got); w != 60 {
		t.Errorf("幅 = %d, want 60", w)
	}
}

// まだ実行していなければ右側を出さない。
func TestSummaryLineWithoutUpdated(t *testing.T) {
	t.Parallel()

	got := SummaryLine(SummaryView{}, "", 60, plainStyles())
	if strings.Contains(got, "最終実行") {
		t.Errorf("実行前なのに最終実行が出ている: %q", got)
	}
}

func TestSummaryViewTotalNonZero(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		in   SummaryView
		want bool
	}{
		"未実行":     {in: SummaryView{}, want: false},
		"OK だけ":   {in: SummaryView{OK: 1}, want: true},
		"SKIP だけ": {in: SummaryView{Skip: 1}, want: true},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got := tt.in.TotalNonZero(); got != tt.want {
				t.Errorf("TotalNonZero() = %v, want %v", got, tt.want)
			}
		})
	}
}

// 装飾なしの表記は色を持たない文脈（テスト・状態行）で使う。
func TestSummaryViewString(t *testing.T) {
	t.Parallel()

	got := SummaryView{OK: 2, Warn: 1, Fail: 0, Skip: 3}.String()
	want := "OK 2  WARN 1  FAIL 0  SKIP 3"
	if got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}
