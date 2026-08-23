package setup_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/runner"
	"github.com/ousiassllc/gsr-helper/internal/setup"
)

func TestPlanRemoveIssuesUninstallThenRemove(t *testing.T) {
	t.Parallel()

	r := testRunner("build01-3", "/opt/runners/build01-3", "actions.runner.foo.build01-3.service", true, false)
	p, err := setup.PlanRemove(setup.RemoveSpec{Runners: []runner.Runner{r}})
	if err != nil {
		t.Fatalf("err = %v", err)
	}

	want := []string{"./svc.sh stop", "./svc.sh uninstall", "./config.sh remove --token ***"}
	if got := p.Units[0].CommandLines(); !slices.Equal(got, want) {
		t.Errorf("コマンド全文:\n got: %v\nwant: %v", got, want)
	}
	if p.NeedsTarball {
		t.Error("削除に tarball は不要である")
	}
	if !p.NeedsToken {
		t.Error("削除には remove token が要る")
	}
}

func TestPlanRemoveSkipsServiceStepsWithoutUnit(t *testing.T) {
	t.Parallel()

	r := testRunner("build01-9", "/opt/runners/build01-9", "", false, false)
	p, err := setup.PlanRemove(setup.RemoveSpec{Runners: []runner.Runner{r}})
	if err != nil {
		t.Fatalf("err = %v", err)
	}

	want := []string{"./config.sh remove --token ***"}
	if got := p.Units[0].CommandLines(); !slices.Equal(got, want) {
		t.Errorf("サービス未登録なら svc.sh を発行しないこと:\n got: %v\nwant: %v", got, want)
	}
}

func TestPlanRemoveKeepsDirectoryAndWarnsOnBusy(t *testing.T) {
	t.Parallel()

	busy := testRunner("build01-3", "/opt/runners/build01-3", "u1.service", true, true)
	idle := testRunner("build01-4", "/opt/runners/build01-4", "u2.service", true, false)

	p, err := setup.PlanRemove(setup.RemoveSpec{Runners: []runner.Runner{busy, idle}})
	if err != nil {
		t.Fatalf("err = %v", err)
	}

	notes := strings.Join(p.Notes, "\n")
	if !strings.Contains(notes, "runner ディレクトリは削除されません") {
		t.Errorf("ディレクトリを残す旨が無い:\n%s", notes)
	}
	for _, d := range []string{"/opt/runners/build01-3", "/opt/runners/build01-4"} {
		if !strings.Contains(notes, d) {
			t.Errorf("残るディレクトリのパス %q が示されていない:\n%s", d, notes)
		}
	}

	warn := strings.Join(p.Warnings, "\n")
	if !strings.Contains(warn, "ジョブ実行中: build01-3") {
		t.Errorf("ジョブ実行中の警告が無い:\n%s", warn)
	}
	if !strings.Contains(warn, "ドレイン停止") {
		t.Errorf("ドレイン停止を促していない:\n%s", warn)
	}
	if strings.Contains(warn, "build01-4") {
		t.Errorf("idle の runner を警告に含めている:\n%s", warn)
	}
}

func TestPlanRemoveRejectsEmptyTargets(t *testing.T) {
	t.Parallel()

	if _, err := setup.PlanRemove(setup.RemoveSpec{Runners: nil}); !errors.Is(err, setup.ErrNoTargets) {
		t.Errorf("err = %v, want ErrNoTargets", err)
	}
}

func TestPlanUpdatePreservesStateFilesAndRestoresOnlyRunning(t *testing.T) {
	t.Parallel()

	running := testRunner("build01-1", "/opt/runners/build01-1", "u1.service", true, false)
	stopped := testRunner("build01-2", "/opt/runners/build01-2", "u2.service", false, false)

	p, err := setup.PlanUpdate(setup.UpdateSpec{
		Runners: []runner.Runner{running, stopped},
		Version: "2.311.0",
	})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if p.NeedsToken {
		t.Error("バージョン更新に短命トークンは要らない")
	}

	// 起動していた台: ドレイン停止 → 展開 → 起動
	gotPhases := phases(p.Units[0])
	if want := []string{"ドレイン停止", "展開", "起動"}; !slices.Equal(gotPhases, want) {
		t.Errorf("起動中の手順 = %v, want %v", gotPhases, want)
	}
	// 停止していた台: 展開のみ（元の状態へ戻すので起動しない）
	if want := []string{"展開"}; !slices.Equal(phases(p.Units[1]), want) {
		t.Errorf("停止中の手順 = %v, want %v", phases(p.Units[1]), want)
	}

	keep := findExtract(t, p.Units[0])
	for _, name := range []string{".runner", ".credentials", ".env", ".path", "_work", "_diag"} {
		if !slices.Contains(keep, name) {
			t.Errorf("保持対象に %q が無い（FR-21）: %v", name, keep)
		}
	}
}

func TestPlanUpdateExcludesRunningRunnersItCannotStop(t *testing.T) {
	t.Parallel()

	standalone := testRunner("run-sh-1", "/opt/runners/run-sh-1", "", true, false)
	standalone.Managed = runner.ManagedStandalone
	ok := testRunner("build01-1", "/opt/runners/build01-1", "u1.service", true, false)

	p, err := setup.PlanUpdate(setup.UpdateSpec{
		Runners: []runner.Runner{standalone, ok},
		Version: "2.311.0",
	})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if want := []string{"build01-1"}; !slices.Equal(p.Names(), want) {
		t.Errorf("対象 = %v, want %v（停止手段の無い稼働中は外す）", p.Names(), want)
	}
	if !strings.Contains(strings.Join(p.Warnings, "\n"), "run-sh-1") {
		t.Errorf("外した runner を警告していない: %v", p.Warnings)
	}
}

func TestPlanUpdateRejectsWhenNothingIsUpdatable(t *testing.T) {
	t.Parallel()

	r := testRunner("run-sh-1", "/opt/runners/run-sh-1", "", true, false)
	r.Managed = runner.ManagedStandalone

	_, err := setup.PlanUpdate(setup.UpdateSpec{Runners: []runner.Runner{r}, Version: "2.311.0"})
	if !errors.Is(err, setup.ErrNoTargets) {
		t.Errorf("err = %v, want ErrNoTargets", err)
	}
}
