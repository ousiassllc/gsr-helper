package hostres

import (
	"context"
	"errors"
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/disk"
	"github.com/ousiassllc/gsr-helper/internal/doctor/check"
	"github.com/ousiassllc/gsr-helper/internal/runner"
)

// statfs は実ホストの空き容量を返すため、閾値の判定を外から検査できない。
// 差し替え口（fsCheck.stat）を内部テストから直接使う。
func stats(usedPct, inodePct int) disk.Stats {
	const totalBytes = int64(1000)
	const totalInodes = int64(1000)
	used := int64(usedPct) * totalBytes / 100
	usedI := int64(inodePct) * totalInodes / 100
	return disk.Stats{
		Path:        "",
		TotalBytes:  totalBytes,
		UsedBytes:   used,
		AvailBytes:  totalBytes - used,
		TotalInodes: totalInodes,
		UsedInodes:  usedI,
		FreeInodes:  totalInodes - usedI,
	}
}

func TestFSCheckThresholds(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		usedPct  int
		inodePct int
		want     check.Status
	}{
		"余裕がある":            {usedPct: 50, inodePct: 10, want: check.OK},
		"容量が閾値の手前":         {usedPct: 79, inodePct: 10, want: check.OK},
		"容量が 80% で注意":      {usedPct: 80, inodePct: 10, want: check.Warn},
		"容量が 90% で異常":      {usedPct: 90, inodePct: 10, want: check.Fail},
		"inode だけが逼迫しても拾う": {usedPct: 10, inodePct: 95, want: check.Fail},
		"重い方の判定を採る":        {usedPct: 85, inodePct: 95, want: check.Fail},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			c := fsCheck{stat: func(string) (disk.Stats, error) {
				return stats(tt.usedPct, tt.inodePct), nil
			}}
			got := c.Run(context.Background(), check.Input{})
			if len(got) == 0 {
				t.Fatal("結果が空（/tmp の行が必ず出るはず）")
			}
			if got[0].Status != tt.want {
				t.Errorf("Status = %v, want %v（Detail: %s）", got[0].Status, tt.want, got[0].Detail)
			}
		})
	}
}

// statfs に失敗したパスは FAIL ではなく SKIP。測れないことはホストの不備ではない。
func TestFSCheckSkipsUnreadablePath(t *testing.T) {
	t.Parallel()

	c := fsCheck{stat: func(string) (disk.Stats, error) {
		return disk.Stats{}, errors.New("取得できません")
	}}
	got := c.Run(context.Background(), check.Input{})
	if len(got) == 0 || got[0].Status != check.Skip {
		t.Fatalf("Status = %+v, want SKIP", got)
	}
}

// runner のディレクトリと /tmp を重複なく見る。並びは検出順で固定する。
func TestFSCheckTargets(t *testing.T) {
	t.Parallel()

	var seen []string
	c := fsCheck{stat: func(path string) (disk.Stats, error) {
		seen = append(seen, path)
		return stats(10, 10), nil
	}}
	in := check.Input{Runners: []runner.Runner{
		{Dir: "/opt/runners/a", WorkDir: "/data/work", Config: runner.Config{AgentName: "a"}},
		{Dir: "/opt/runners/b", WorkDir: "/opt/runners/a", Config: runner.Config{AgentName: "b"}},
	}}

	got := c.Run(context.Background(), in)
	want := []string{"/opt/runners/a", "/data/work", "/opt/runners/b", "/tmp"}
	if len(seen) != len(want) {
		t.Fatalf("集計したパス = %q, want %q（重複を除いていない）", seen, want)
	}
	for i := range want {
		if seen[i] != want[i] {
			t.Errorf("%d 番目のパス = %q, want %q", i, seen[i], want[i])
		}
	}
	// runner に紐付く行は TARGET 列に runner 名を出す。/tmp はホスト全体。
	if got[0].Target != "a" {
		t.Errorf("Target = %q, want %q", got[0].Target, "a")
	}
	if got[len(got)-1].Target != "" {
		t.Errorf("/tmp の Target = %q, want 空", got[len(got)-1].Target)
	}
}
