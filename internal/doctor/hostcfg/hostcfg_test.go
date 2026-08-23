package hostcfg_test

import (
	"context"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/appconfig"
	"github.com/ousiassllc/gsr-helper/internal/doctor/check"
	"github.com/ousiassllc/gsr-helper/internal/doctor/hostcfg"
	"github.com/ousiassllc/gsr-helper/internal/exec"
	"github.com/ousiassllc/gsr-helper/internal/runner"
	"github.com/ousiassllc/gsr-helper/internal/runner/systemd"
)

func checkByID(t *testing.T, id string) check.Check {
	t.Helper()

	for _, c := range hostcfg.Checks() {
		if c.ID() == id {
			return c
		}
	}
	t.Fatalf("項目 %q が Checks() に無い", id)
	return nil
}

func run(t *testing.T, id string, in check.Input) []check.Result {
	t.Helper()
	return checkByID(t, id).Run(context.Background(), in)
}

func only(t *testing.T, got []check.Result) check.Result {
	t.Helper()

	if len(got) != 1 {
		t.Fatalf("件数 = %d, want 1（%+v）", len(got), got)
	}
	return got[0]
}

func byTarget(t *testing.T, got []check.Result, target string) check.Result {
	t.Helper()

	for _, r := range got {
		if r.Target == target {
			return r
		}
	}
	t.Fatalf("対象 %q の結果が無い（%+v）", target, got)
	return check.Result{}
}

func okResult(stdout string) exec.Result {
	return exec.Result{Stdout: []byte(stdout), Stderr: nil, ExitCode: 0}
}

func lookOnly(names ...string) func(string) (string, error) {
	set := make(map[string]bool, len(names))
	for _, n := range names {
		set[n] = true
	}
	return func(name string) (string, error) {
		if set[name] {
			return "/usr/bin/" + name, nil
		}
		return "", os.ErrNotExist
	}
}

// unitFixture は systemctl show が返すユニット 1 つぶんの値。
type unitFixture struct {
	load       string
	active     string
	workingDir string
	restart    string
	env        string
}

// systemctlFake は list-units と show に応える Executor を返す。
//
// systemd.Scan は list-units → ユニットごとの show を発行する。孤児と重複の
// 判定はその戻りに乗るので、実ホストの systemd に依存させずに検査できる。
func systemctlFake(units map[string]unitFixture, order []string) *exec.Fake {
	f := exec.NewFake()
	f.SetFunc(func(_ string, args []string) (exec.Result, error) {
		if len(args) > 0 && args[0] == "list-units" {
			return okResult(strings.Join(order, "\n") + "\n"), nil
		}
		if len(args) > 1 && args[0] == "show" {
			u, ok := units[args[1]]
			if !ok {
				return okResult(""), nil
			}
			return okResult(
				"Id=" + args[1] + "\n" +
					"LoadState=" + u.load + "\n" +
					"ActiveState=" + u.active + "\n" +
					"SubState=running\n" +
					"UnitFileState=enabled\n" +
					"WorkingDirectory=" + u.workingDir + "\n" +
					"MainPID=100\n" +
					"User=runner\n" +
					"Restart=" + u.restart + "\n" +
					"Environment=" + u.env + "\n",
			), nil
		}
		return okResult(""), nil
	})
	return f
}

func newRunner(name, dir, unit string) runner.Runner {
	r := runner.Runner{
		Dir:      dir,
		Config:   runner.Config{AgentName: name},
		UnitName: unit,
		Managed:  runner.ManagedSystemd,
	}
	if unit != "" {
		r.Svc = &systemd.State{Unit: unit, Load: "loaded", Active: "active", WorkingDir: dir}
	}
	return r
}

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
