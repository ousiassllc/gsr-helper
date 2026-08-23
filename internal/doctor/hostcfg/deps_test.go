package hostcfg_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/doctor/check"
	"github.com/ousiassllc/gsr-helper/internal/doctor/hostcfg"
	"github.com/ousiassllc/gsr-helper/internal/exec"
)

// この 3 分類は起動時の自動判定（FR-44）に入れない。
func TestHostCfgChecksAreNotRunAtStartup(t *testing.T) {
	t.Parallel()

	want := map[string]string{
		"deps.commands":      check.CatDeps,
		"systemd.unit":       check.CatSystemd,
		"consistency.orphan": check.CatConsistency,
		"consistency.units":  check.CatConsistency,
	}
	var ids []string
	for _, c := range hostcfg.Checks() {
		ids = append(ids, c.ID())
		if c.Startup() {
			t.Errorf("%s が起動時の対象になっている", c.ID())
		}
		if got := c.Category(); got != want[c.ID()] {
			t.Errorf("%s の Category = %q, want %q", c.ID(), got, want[c.ID()])
		}
	}
	slices.Sort(ids)
	wantIDs := []string{"consistency.orphan", "consistency.units", "deps.commands", "systemd.unit"}
	if !slices.Equal(ids, wantIDs) {
		t.Errorf("項目 = %q, want %q", ids, wantIDs)
	}
}

// git の欠落は FAIL、docker / node は WARN。必須かどうかがコマンドで違う。
func TestDepsSeverity(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		present []string
		want    map[string]check.Status
	}{
		"すべてある": {
			present: []string{"git", "docker", "node"},
			want:    map[string]check.Status{"git": check.OK, "docker": check.OK, "node": check.OK},
		},
		"すべて無い": {
			present: nil,
			want:    map[string]check.Status{"git": check.Fail, "docker": check.Warn, "node": check.Warn},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			f := exec.NewFake()
			f.SetFunc(func(name string, _ []string) (exec.Result, error) {
				return okResult(name + " version 1.0.0"), nil
			})
			in := check.Input{Exec: f, LookPath: lookOnly(tt.present...)}

			got := run(t, "deps.commands", in)
			if len(got) != 3 {
				t.Fatalf("件数 = %d, want 3", len(got))
			}
			for _, r := range got {
				cmd := strings.Fields(r.Summary)[0]
				if r.Status != tt.want[cmd] {
					t.Errorf("%s の Status = %v, want %v", cmd, r.Status, tt.want[cmd])
				}
			}
		})
	}
}

// gcc は検査しない。runner-host-setup.md が検査対象から明示的に外している。
func TestDepsDoesNotCheckCCompiler(t *testing.T) {
	t.Parallel()

	f := exec.NewFake()
	f.SetFunc(func(name string, _ []string) (exec.Result, error) { return okResult(name), nil })
	got := run(t, "deps.commands", check.Input{Exec: f, LookPath: lookOnly("git", "docker", "node")})

	for _, r := range got {
		if strings.Contains(r.Summary, "gcc") || strings.Contains(r.Summary, "build-essential") {
			t.Errorf("C コンパイラを検査している: %s", r.Summary)
		}
	}
}
