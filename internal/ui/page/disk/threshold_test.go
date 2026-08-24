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
		inodes int
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
		// Issue #127: inode の枯渇は容量に余裕があっても起きる。doctor は
		// worse(band(used), band(inodes)) で判定するので、この行も重い方を見る。
		{name: "inode だけが閾値に達しても超過", used: 10, inodes: 85, warn: 80, want: true},
		{name: "inode が閾値に達したら超過", used: 10, inodes: 80, warn: 80, want: true},
		{name: "どちらも閾値未満なら超過しない", used: 79, inodes: 79, warn: 80, want: false},
		{name: "取得に失敗した行は inode でも超過にしない", failed: true, used: 0, inodes: 99, warn: 80, want: false},
		{name: "閾値が未設定なら inode でも超過にしない", used: 0, inodes: 99, warn: 0, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := warnExceeded(tt.failed, tt.used, tt.inodes, tt.warn); got != tt.want {
				t.Errorf("warnExceeded(%v, %d, %d, %d) = %v, want %v",
					tt.failed, tt.used, tt.inodes, tt.warn, got, tt.want)
			}
		})
	}
}

// TestSummaryLineWarnsOnInodeOnly は inode だけが逼迫したファイルシステムで
// 要約行に印が出ることを、部品（molecule.FSSummaryLine）まで通して確かめる
// （Issue #127）。
//
// doctor のリソース診断は同じ数字に対して WARN を出す（internal/doctor/hostres の
// judge が重い方を採る）。この検証が落ちると、Disk タブが「inode 85%」と自分で
// 印字しながら黙る状態へ戻ったということである。
func TestSummaryLineWarnsOnInodeOnly(t *testing.T) {
	st, _ := baseState()
	st.Disk.Thresholds = appconfig.DiskThresholds{Warn: 80, Critical: 90}

	m := newModel(t, st)
	// ディスクは 10%、inode は 85%。
	m.stats = disk.Stats{
		Path: "/", UsedBytes: 10, AvailBytes: 90, TotalBytes: 100,
		UsedInodes: 85, FreeInodes: 15,
	}

	view := m.summaryView()
	if view.UsedPercent >= 80 {
		t.Fatalf("前提が崩れている: ディスク使用率 = %d%%, want 閾値未満", view.UsedPercent)
	}
	if view.InodePercent < 80 {
		t.Fatalf("前提が崩れている: inode 使用率 = %d%%, want 閾値以上", view.InodePercent)
	}
	if !view.Warn {
		t.Error("inode だけが閾値を超えた行で Warn が立っていない")
	}
	if body := m.View().Content; !strings.Contains(body, "警告閾値超過") {
		t.Errorf("要約行に警告が出ていない:\n%s", body)
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
