package disk

import (
	"path/filepath"
	"testing"
)

// TestFSStats は実在のディレクトリで statfs が読めることを確かめる。
// 値そのものは環境で変わるため、整合が取れていること（総容量 > 0、
// 使用 + 空き ≤ 総容量、使用率が 0〜100）だけを固定する。
func TestFSStats(t *testing.T) {
	dir := t.TempDir()

	s, err := FSStats(dir)
	if err != nil {
		t.Fatalf("FSStats がエラーを返した: %v", err)
	}
	if s.Path != dir {
		t.Errorf("Path = %q, want %q", s.Path, dir)
	}
	if s.TotalBytes <= 0 {
		t.Errorf("TotalBytes = %d, want 正の値", s.TotalBytes)
	}
	if s.UsedBytes < 0 || s.AvailBytes < 0 || s.UsedBytes+s.AvailBytes > s.TotalBytes {
		t.Errorf("使用 %d + 空き %d が総容量 %d を超えている", s.UsedBytes, s.AvailBytes, s.TotalBytes)
	}
	if p := s.UsedPercent(); p < 0 || p > 100 {
		t.Errorf("UsedPercent = %d, want 0〜100", p)
	}
	if s.TotalInodes != s.UsedInodes+s.FreeInodes {
		t.Errorf("inode 総数 %d != 使用 %d + 空き %d", s.TotalInodes, s.UsedInodes, s.FreeInodes)
	}
	if p := s.InodePercent(); p < 0 || p > 100 {
		t.Errorf("InodePercent = %d, want 0〜100", p)
	}
}

func TestFSStatsError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing")

	if _, err := FSStats(path); err == nil {
		t.Fatalf("存在しないパス %q でエラーを返さなかった", path)
	}
}

// TestStatsPercent は 0 除算と端の値を固定する。inode が枯渇しているのに
// 0% と出るような誤表示を防ぐ。
func TestStatsPercent(t *testing.T) {
	tests := []struct {
		name      string
		stats     Stats
		wantUsed  int
		wantInode int
	}{
		{
			name: "ゼロ値は 0%",
			stats: Stats{
				Path: "/", TotalBytes: 0, UsedBytes: 0, AvailBytes: 0,
				TotalInodes: 0, UsedInodes: 0, FreeInodes: 0,
			},
			wantUsed: 0, wantInode: 0,
		},
		{
			name: "予約ブロックは分母に含めない",
			stats: Stats{
				Path: "/", TotalBytes: 100, UsedBytes: 82, AvailBytes: 18,
				TotalInodes: 100, UsedInodes: 34, FreeInodes: 66,
			},
			wantUsed: 82, wantInode: 34,
		},
		{
			name: "満杯は 100%",
			stats: Stats{
				Path: "/", TotalBytes: 50, UsedBytes: 50, AvailBytes: 0,
				TotalInodes: 50, UsedInodes: 50, FreeInodes: 0,
			},
			wantUsed: 100, wantInode: 100,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.stats.UsedPercent(); got != tt.wantUsed {
				t.Errorf("UsedPercent = %d, want %d", got, tt.wantUsed)
			}
			if got := tt.stats.InodePercent(); got != tt.wantInode {
				t.Errorf("InodePercent = %d, want %d", got, tt.wantInode)
			}
		})
	}
}
