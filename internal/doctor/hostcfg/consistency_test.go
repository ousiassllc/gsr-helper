package hostcfg_test

import (
	"strings"
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/appconfig"
	"github.com/ousiassllc/gsr-helper/internal/doctor/check"
	"github.com/ousiassllc/gsr-helper/internal/runner"
)

// 対応する runner の無いユニットは孤児として警告する。
func TestOrphanUnits(t *testing.T) {
	t.Parallel()

	const attached = "actions.runner.acme.build01.service"
	const stray = "actions.runner.acme.gone.service"

	in := check.Input{
		Runners: []runner.Runner{newRunner("build01", "/opt/runners/build01", attached)},
		Caps:    appconfig.Caps{Systemd: true},
		Exec: systemctlFake(map[string]unitFixture{
			attached: {load: "loaded", active: "active", workingDir: "/opt/runners/build01", restart: "always"},
			stray:    {load: "loaded", active: "inactive", workingDir: "/opt/runners/gone", restart: "always"},
		}, []string{attached, stray}),
	}

	got := run(t, "consistency.orphan", in)
	r := byTarget(t, got, stray)
	if r.Status != check.Warn {
		t.Errorf("Status = %v, want %v", r.Status, check.Warn)
	}
	if len(got) != 1 {
		t.Errorf("件数 = %d, want 1（紐付いているユニットを孤児にしていないか）: %+v", len(got), got)
	}
}

// LoadState=not-found（FR-05）と状態不明（screens.md 1.6）は孤児にしない。
// どちらも孤児として出すと誤報になる。
func TestOrphanExcludesNotFoundAndUnknown(t *testing.T) {
	t.Parallel()

	tests := map[string]unitFixture{
		"not-found":           {load: "not-found", active: "inactive", workingDir: "/opt/runners/gone"},
		"状態不明":                {load: "", active: "", workingDir: "/opt/runners/gone"},
		"WorkingDirectory 不明": {load: "loaded", active: "active", workingDir: ""},
	}

	for name, fixture := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			const unit = "actions.runner.acme.gone.service"
			in := check.Input{
				Runners: []runner.Runner{newRunner("build01", "/opt/runners/build01", "")},
				Caps:    appconfig.Caps{Systemd: true},
				Exec:    systemctlFake(map[string]unitFixture{unit: fixture}, []string{unit}),
			}
			got := only(t, run(t, "consistency.orphan", in))
			if got.Status != check.OK {
				t.Errorf("Status = %v, want %v（孤児として報告している）: %s", got.Status, check.OK, got.Detail)
			}
		})
	}
}

// 同一 runner に複数のユニットが対応する状態を検出する。
//
// internal/runner の attachUnits はこの 2 本目を黙って捨てる
// （TestAttachDuplicateUnitIsNotOrphan が固定している）。捨てられた事実を
// 診断として拾い直すのがこの項目の役目である。
func TestDuplicateUnitsAreReported(t *testing.T) {
	t.Parallel()

	const dir = "/opt/runners/build01"
	const attached = "actions.runner.acme.build01.service"
	const dup = "actions.runner.acme.build01-old.service"

	in := check.Input{
		Runners: []runner.Runner{newRunner("build01", dir, attached)},
		Caps:    appconfig.Caps{Systemd: true},
		Exec: systemctlFake(map[string]unitFixture{
			attached: {load: "loaded", active: "active", workingDir: dir, restart: "always"},
			dup:      {load: "loaded", active: "inactive", workingDir: dir, restart: "always"},
		}, []string{attached, dup}),
	}

	got := only(t, run(t, "consistency.units", in))
	if got.Status != check.Warn {
		t.Fatalf("Status = %v, want %v（Detail: %s）", got.Status, check.Warn, got.Detail)
	}
	if !strings.Contains(got.Detail, dup) {
		t.Errorf("重複したユニット名が Detail に無い: %s", got.Detail)
	}
}

// .service に記録された名前と実際に紐付いたユニットの食い違いを検出する。
func TestUnitNameMismatch(t *testing.T) {
	t.Parallel()

	const dir = "/opt/runners/build01"
	const attached = "actions.runner.acme.build01.service"

	r := newRunner("build01", dir, attached)
	r.UnitName = "actions.runner.acme.old-name.service" // .service の記録だけが古い

	in := check.Input{
		Runners: []runner.Runner{r},
		Caps:    appconfig.Caps{Systemd: true},
		Exec: systemctlFake(map[string]unitFixture{
			attached: {load: "loaded", active: "active", workingDir: dir, restart: "always"},
		}, []string{attached}),
	}

	got := only(t, run(t, "consistency.units", in))
	if got.Status != check.Warn {
		t.Fatalf("Status = %v, want %v", got.Status, check.Warn)
	}
	if !strings.Contains(got.Summary, "一致しない") {
		t.Errorf("Summary = %q, want 不一致の旨", got.Summary)
	}
}

// 問題が無ければ OK。
func TestConsistencyOK(t *testing.T) {
	t.Parallel()

	const dir = "/opt/runners/build01"
	const unit = "actions.runner.acme.build01.service"

	in := check.Input{
		Runners: []runner.Runner{newRunner("build01", dir, unit)},
		Caps:    appconfig.Caps{Systemd: true},
		Exec: systemctlFake(map[string]unitFixture{
			unit: {load: "loaded", active: "active", workingDir: dir, restart: "always"},
		}, []string{unit}),
	}

	if got := only(t, run(t, "consistency.units", in)).Status; got != check.OK {
		t.Errorf("Status = %v, want %v", got, check.OK)
	}
}
