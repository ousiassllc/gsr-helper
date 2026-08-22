package molecule

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/ousiassllc/gsr-helper/internal/ui/token"
)

// gib は 1 GiB。期待値を読みやすくするために置く。
const gib = 1024 * 1024 * 1024

func sampleFS() FSSummaryView {
	return FSSummaryView{
		Path:         "/",
		UsedPercent:  82,
		UsedBytes:    410 * gib,
		TotalBytes:   500 * gib,
		InodePercent: 34,
		Warn:         false,
	}
}

// 行の中身は screens.md の Disk タブのモックの 1 行目である。
func TestFSSummaryLine(t *testing.T) {
	got := FSSummaryLine(sampleFS(), 80, plainStyles())
	want := "ファイルシステム  /  使用 82% (410.0G/500.0G)  inode 34%"
	if got != want {
		t.Errorf("FSSummaryLine = %q, want %q", got, want)
	}
}

// 閾値を超えたときは記号と文言を行末に出す。色を使えない端末でも読み取れる。
func TestFSSummaryLineWarn(t *testing.T) {
	v := sampleFS()
	v.Warn = true

	got := FSSummaryLine(v, 80, plainStyles())
	if !strings.HasSuffix(got, token.IconWarn+" 警告閾値超過") {
		t.Errorf("FSSummaryLine = %q, want 行末に %q", got, token.IconWarn+" 警告閾値超過")
	}
	// 同じ意味の記号を 1 行に 2 つ並べない（使用率には添えない）。
	if n := strings.Count(got, token.IconWarn); n != 1 {
		t.Errorf("注意記号の数 = %d, want 1（%q）", n, got)
	}
	// 警告付きでも保証する幅に収まる。収まらないと Join が警告そのものを落とす。
	if w := lipgloss.Width(got); w > token.WidthTarget {
		t.Errorf("警告付きの行幅 = %d, want %d 以下（%q）", w, token.WidthTarget, got)
	}
}

// 未取得の値は「無い」と分かる形にする。0 と読ませない。
func TestFSSummaryLineMissingValues(t *testing.T) {
	tests := map[string]struct {
		view     FSSummaryView
		contains string
		absent   string
	}{
		"総容量が未取得なら容量を出さない": {
			view:     FSSummaryView{Path: "/", UsedPercent: 82, UsedBytes: -1, TotalBytes: 0, InodePercent: 0, Warn: false},
			contains: "使用 82%",
			absent:   "(",
		},
		"マウントポイントが未取得": {
			view:     FSSummaryView{Path: "", UsedPercent: 0, UsedBytes: 0, TotalBytes: 0, InodePercent: 0, Warn: false},
			contains: token.IconNoUnit,
			absent:   "(",
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			got := FSSummaryLine(tt.view, 80, plainStyles())
			if !strings.Contains(got, tt.contains) {
				t.Errorf("FSSummaryLine = %q, want %q を含む", got, tt.contains)
			}
			if strings.Contains(got, tt.absent) {
				t.Errorf("FSSummaryLine = %q, want %q を含まない", got, tt.absent)
			}
		})
	}
}

// 幅に収まらない場合も行が幅を超えない（中略して落とす）。
func TestFSSummaryLineFitsWidth(t *testing.T) {
	v := sampleFS()
	v.Warn = true
	v.Path = "/var/lib/very/long/mount/point"

	for _, width := range []int{token.WidthTarget, 70, token.WidthMin, 20, 1} {
		if got := lipgloss.Width(FSSummaryLine(v, width, plainStyles())); got > width {
			t.Errorf("幅 %d の行幅 = %d, want %d 以下", width, got, width)
		}
	}
}
