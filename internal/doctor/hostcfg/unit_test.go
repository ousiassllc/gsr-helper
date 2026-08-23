package hostcfg_test

import (
	"strings"
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/appconfig"
	"github.com/ousiassllc/gsr-helper/internal/doctor/check"
	"github.com/ousiassllc/gsr-helper/internal/runner"
)

// WorkingDirectory の食い違いは FAIL、Restart の欠落は WARN。
func TestSystemdUnit(t *testing.T) {
	t.Parallel()

	const dir = "/opt/runners/build01"
	const unit = "actions.runner.acme.build01.service"

	tests := map[string]struct {
		fixture unitFixture
		want    check.Status
	}{
		"整合している": {
			fixture: unitFixture{load: "loaded", active: "active", workingDir: dir, restart: "always"},
			want:    check.OK,
		},
		"Restart が無い": {
			fixture: unitFixture{load: "loaded", active: "active", workingDir: dir, restart: "no"},
			want:    check.Warn,
		},
		"Restart が空": {
			fixture: unitFixture{load: "loaded", active: "active", workingDir: dir, restart: ""},
			want:    check.Warn,
		},
		"WorkingDirectory が違う": {
			fixture: unitFixture{load: "loaded", active: "active", workingDir: "/opt/runners/other", restart: "always"},
			want:    check.Fail,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			in := check.Input{
				Runners: []runner.Runner{newRunner("build01", dir, unit)},
				Caps:    appconfig.Caps{Systemd: true},
				Exec:    systemctlFake(map[string]unitFixture{unit: tt.fixture}, []string{unit}),
			}
			got := only(t, run(t, "systemd.unit", in))
			if got.Status != tt.want {
				t.Errorf("Status = %v, want %v（Detail: %s）", got.Status, tt.want, got.Detail)
			}
		})
	}
}

// Environment の是非は判定せず、値を見せて運用者に委ねる。
func TestSystemdUnitShowsEnvironmentWithoutJudging(t *testing.T) {
	t.Parallel()

	const dir = "/opt/runners/build01"
	const unit = "actions.runner.acme.build01.service"
	in := check.Input{
		Runners: []runner.Runner{newRunner("build01", dir, unit)},
		Caps:    appconfig.Caps{Systemd: true},
		Exec: systemctlFake(map[string]unitFixture{
			unit: {load: "loaded", active: "active", workingDir: dir, restart: "always", env: "https_proxy=http://p:3128"},
		}, []string{unit}),
	}

	got := only(t, run(t, "systemd.unit", in))
	if got.Status != check.OK {
		t.Errorf("Status = %v, want %v（Environment だけで判定を落とさない）", got.Status, check.OK)
	}
	if !strings.Contains(got.Detail, "https_proxy=http://p:3128") {
		t.Errorf("Environment が Detail に出ていない: %s", got.Detail)
	}
}

// systemctl が無ければ SKIP。
func TestSystemdChecksSkipWithoutSystemctl(t *testing.T) {
	t.Parallel()

	in := check.Input{Caps: appconfig.Caps{Systemd: false}}
	for _, id := range []string{"systemd.unit", "consistency.orphan", "consistency.units"} {
		t.Run(id, func(t *testing.T) {
			t.Parallel()
			if got := only(t, run(t, id, in)).Status; got != check.Skip {
				t.Errorf("Status = %v, want %v", got, check.Skip)
			}
		})
	}
}
