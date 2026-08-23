package hostcfg_test

import (
	"strings"
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/appconfig"
	"github.com/ousiassllc/gsr-helper/internal/doctor/check"
	"github.com/ousiassllc/gsr-helper/internal/exec"
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
	if !strings.Contains(got.Summary, ".service") || !strings.Contains(got.Summary, "一致しない") {
		t.Errorf("Summary = %q, want .service の記録との不一致である旨", got.Summary)
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

// ユニット一覧を取得できないときは OK ではなく SKIP を返す。
//
// systemctl が在っても list-units は失敗しうるし、Caps.Systemd は LookPath 由来で
// Exec の配布とは独立に真になる。どちらも「一覧が 0 件」と区別できないと、検査を
// していないのに緑の行が並ぶ。
func TestConsistencySkipsWhenUnitsUnavailable(t *testing.T) {
	t.Parallel()

	const dir = "/opt/runners/build01"
	const unit = "actions.runner.acme.build01.service"

	tests := map[string]struct {
		exec exec.Executor
	}{
		"Exec が配られていない":    {exec: nil},
		"list-units が失敗する": {exec: systemctlListUnitsFails()},
	}

	for name, tt := range tests {
		for _, id := range []string{"consistency.orphan", "consistency.units"} {
			t.Run(name+"/"+id, func(t *testing.T) {
				t.Parallel()

				in := check.Input{
					Runners: []runner.Runner{newRunner("build01", dir, unit)},
					Caps:    appconfig.Caps{Systemd: true},
					Exec:    tt.exec,
				}
				got := only(t, run(t, id, in))
				if got.Status != check.Skip {
					t.Errorf("Status = %v, want %v（検査していないのに判定を出している）: %s",
						got.Status, check.Skip, got.Detail)
				}
			})
		}
	}
}

// 突き合わせる材料が欠けている runner は OK ではなく SKIP。
//
// .service を持たない runner（svc.sh を使わず登録したもの）は記録値が空になるが、
// それは「ユニット名が一致している」ことの根拠にならない。
func TestUnitNameWithoutBaselineIsSkipped(t *testing.T) {
	t.Parallel()

	const dir = "/opt/runners/build01"
	const unit = "actions.runner.acme.build01.service"

	withoutRecord := newRunner("build01", dir, unit)
	withoutRecord.UnitName = "" // .service が無く記録値を持たない

	tests := map[string]struct {
		runner runner.Runner
		want   check.Status
	}{
		".service の記録が無い": {runner: withoutRecord, want: check.Skip},
		// ディレクトリを分けるのは、同じ dir を指すユニットがあると
		// 重複の判定（そちらが先）に入ってしまうためである。
		"記録も紐付きも無い": {runner: newRunner("build02", "/opt/runners/build02", ""), want: check.Skip},
		"記録と紐付きが一致": {runner: newRunner("build01", dir, unit), want: check.OK},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			in := check.Input{
				Runners: []runner.Runner{tt.runner},
				Caps:    appconfig.Caps{Systemd: true},
				Exec: systemctlFake(map[string]unitFixture{
					unit: {load: "loaded", active: "active", workingDir: dir, restart: "always"},
				}, []string{unit}),
			}

			got := only(t, run(t, "consistency.units", in))
			if got.Status != tt.want {
				t.Errorf("Status = %v, want %v: %s", got.Status, tt.want, got.Detail)
			}
		})
	}
}

// 重複ユニットの検出は .service の有無と無関係に成立する。
// 判定材料なしの SKIP より先に判定すること。
func TestDuplicateUnitsReportedWithoutServiceRecord(t *testing.T) {
	t.Parallel()

	const dir = "/opt/runners/build01"
	const attached = "actions.runner.acme.build01.service"
	const dup = "actions.runner.acme.build01-old.service"

	r := newRunner("build01", dir, attached)
	r.UnitName = "" // .service が無くても重複は分かる

	in := check.Input{
		Runners: []runner.Runner{r},
		Caps:    appconfig.Caps{Systemd: true},
		Exec: systemctlFake(map[string]unitFixture{
			attached: {load: "loaded", active: "active", workingDir: dir, restart: "always"},
			dup:      {load: "loaded", active: "inactive", workingDir: dir, restart: "always"},
		}, []string{attached, dup}),
	}

	got := only(t, run(t, "consistency.units", in))
	if got.Status != check.Warn {
		t.Fatalf("Status = %v, want %v（SKIP が重複の検出を隠している）: %s",
			got.Status, check.Warn, got.Detail)
	}
	if !strings.Contains(got.Detail, dup) {
		t.Errorf("重複したユニット名が Detail に無い: %s", got.Detail)
	}
}
