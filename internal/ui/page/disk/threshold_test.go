package disk

import (
	"errors"
	"strings"
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/appconfig"
	"github.com/ousiassllc/gsr-helper/internal/disk"
)

// 閾値超過の表示（Issue #72）を検証する。判定の境界と縮退は warnExceeded を直に、
// 部品（molecule.FSSummaryLine）と繋がっていることは要約行の文言で確かめる。

func TestWarnExceeded(t *testing.T) {
	tests := []struct {
		name   string
		failed bool
		used   int
		warn   int
		want   bool
	}{
		{name: "閾値に達したら超過", used: 80, warn: 80, want: true},
		{name: "閾値を超えたら超過", used: 82, warn: 80, want: true},
		{name: "閾値未満なら超過しない", used: 79, warn: 80, want: false},
		{name: "既定の 80 に固定されていない", used: 79, warn: 50, want: true},
		{name: "取得に失敗した行は超過にしない", failed: true, used: 99, warn: 80, want: false},
		{name: "閾値が未設定なら超過にしない", used: 99, warn: 0, want: false},
		{name: "閾値が負なら超過にしない", used: 99, warn: -1, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := warnExceeded(tt.failed, tt.used, tt.warn); got != tt.want {
				t.Errorf("warnExceeded(%v, %d, %d) = %v, want %v", tt.failed, tt.used, tt.warn, got, tt.want)
			}
		})
	}
}

// 要約行が設定した閾値に従う（既定の 80 / 90 に固定されていない）。
//
// 取得に失敗した行で警告を出さないことも同じ表で見る。失敗時の使用率は「値なし」と
// して描かれるため、警告だけが残ると「使用 - なのに閾値超過」という読めない行になる。
func TestSummaryLineFollowsConfiguredThreshold(t *testing.T) {
	tests := []struct {
		name    string
		warn    int
		used    int64
		statErr error
		want    bool
	}{
		{name: "設定した閾値を超えたら出す", warn: 50, used: 60, want: true},
		{name: "閾値未満なら出さない", warn: 80, used: 10, want: false},
		{name: "取得に失敗したら出さない", warn: 80, used: 99, statErr: errors.New("statfs に失敗")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st, _ := baseState()
			st.Disk.Thresholds = appconfig.DiskThresholds{Warn: tt.warn, Critical: 90}

			m := newModel(t, st)
			m.stats = disk.Stats{Path: "/", UsedBytes: tt.used, AvailBytes: 100 - tt.used, TotalBytes: 100}
			m.statsErr = tt.statErr

			if got := m.summaryView().Warn; got != tt.want {
				t.Fatalf("Warn = %v, want %v（使用率 %d%% / 閾値 %d%%）", got, tt.want, tt.used, tt.warn)
			}
			// NO_COLOR でも記号ではなく文言で判別できることを本文で確かめる。
			body := m.View().Content
			if got := strings.Contains(body, "警告閾値超過"); got != tt.want {
				t.Errorf("要約行の警告 = %v, want %v:\n%s", got, tt.want, body)
			}
		})
	}
}
