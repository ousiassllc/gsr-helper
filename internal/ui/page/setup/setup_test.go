package setup_test

import (
	"strings"
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/runner"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/pagetest"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/setup"
)

func TestDeleteRequestShowsConfirmBeforeAnyCommand(t *testing.T) {
	t.Parallel()

	st := state(t, pagetest.SampleRunner())
	f := fakeOf(t, st)
	m := newModel(t, st)

	m = send(t, m, page.SetupRequestMsg{
		Op: page.SetupRemove, Runners: []runner.Runner{pagetest.SampleRunner()},
	})

	got := view(m)
	if !strings.Contains(got, "削除の確認") {
		t.Fatalf("確認ダイアログが出ていない:\n%s", got)
	}
	if len(f.Calls()) != 0 {
		t.Errorf("承認の前にコマンドを発行している: %v", f.Calls())
	}
}

func TestDeleteConfirmShowsPlannedCommandsWithMaskedToken(t *testing.T) {
	t.Parallel()

	st := state(t, pagetest.SampleRunner())
	m := newModel(t, st)
	m = send(t, m, page.SetupRequestMsg{
		Op: page.SetupRemove, Runners: []runner.Runner{pagetest.SampleRunner()},
	})

	got := view(m)
	for _, want := range []string{
		"./svc.sh stop", "./svc.sh uninstall", "./config.sh remove --token ***",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("実行するコマンド %q が出ていない:\n%s", want, got)
		}
	}
	// FR-18: runner ディレクトリは残す。残る場所を示す。
	if !strings.Contains(got, "runner ディレクトリは削除されません") {
		t.Errorf("ディレクトリを残す旨が出ていない:\n%s", got)
	}
}

func TestDeleteOfBusyRunnerIsBlocked(t *testing.T) {
	t.Parallel()

	busy := pagetest.BusyRunner()
	st := state(t, busy)
	f := fakeOf(t, st)
	m := newModel(t, st)

	m = send(t, m, page.SetupRequestMsg{Op: page.SetupRemove, Runners: []runner.Runner{busy}})

	if strings.Contains(view(m), "削除の確認") {
		t.Error("ジョブ実行中の runner の削除で確認が開いている（先にドレイン停止を促すこと）")
	}
	if len(f.Calls()) != 0 {
		t.Errorf("コマンドを発行している: %v", f.Calls())
	}

	chrome := chromeOf(t, m)
	if !strings.Contains(chrome.Status, "ドレイン停止") {
		t.Errorf("状態行 = %q, ドレイン停止を促すこと", chrome.Status)
	}
}

func TestCancelingConfirmDoesNotRun(t *testing.T) {
	t.Parallel()

	st := state(t, pagetest.SampleRunner())
	f := fakeOf(t, st)
	m := newModel(t, st)
	m = send(t, m, page.SetupRequestMsg{
		Op: page.SetupRemove, Runners: []runner.Runner{pagetest.SampleRunner()},
	})
	m = send(t, m, pagetest.Press("n"))

	if strings.Contains(view(m), "削除の確認") {
		t.Error("キャンセルしても確認が閉じていない")
	}
	if len(f.Calls()) != 0 {
		t.Errorf("キャンセルしたのにコマンドを発行している: %v", f.Calls())
	}
}

func TestApprovalStartsTheRun(t *testing.T) {
	t.Parallel()

	st := state(t, pagetest.SampleRunner())
	m := newModel(t, st)
	m = send(t, m, page.SetupRequestMsg{
		Op: page.SetupRemove, Runners: []runner.Runner{pagetest.SampleRunner()},
	})
	m = send(t, m, pagetest.Press("y"))

	// 承認すると確認は閉じ、進捗表示へ移る。
	got := view(m)
	if strings.Contains(got, "実行しますか?") {
		t.Errorf("承認したのに確認が残っている:\n%s", got)
	}
	if !strings.Contains(got, "削除中…") && !strings.Contains(got, "完了") {
		t.Errorf("進捗表示へ移っていない:\n%s", got)
	}
	if !strings.Contains(got, pagetest.SampleRunner().Name()) {
		t.Errorf("進捗表示に対象が出ていない:\n%s", got)
	}
}

// 登録の Cmd は最初の共有状態で 1 度だけ親へ流れる。2 度流すとモーダルが
// 二重に登録され、Overlay が種類の重複で panic する。
func TestRegisterCmdReachesParentOnlyOnce(t *testing.T) {
	t.Parallel()

	st := state(t, pagetest.SampleRunner())
	m := setup.New(0, st)

	if cmd := m.Init(); cmd != nil {
		t.Error("Init が Cmd を返している（親はタブの Init を呼ばない）")
	}

	_, first := m.Update(st)
	if first == nil {
		t.Fatal("最初の共有状態で登録の Cmd が流れていない")
	}

	next, _ := m.Update(st)
	second, ok := next.(setup.Model)
	if !ok {
		t.Fatalf("型 = %T", next)
	}
	_, again := second.Update(st)

	// 2 度目に流れるのは ChromeMsg などの毎回の Cmd だけで、登録は含まれない。
	// 登録が再び流れていれば Overlay の二重登録で panic する。
	pagetest.RunAll(again)
}

// ジョブ実行中の runner を含む削除は、対象ごとに判定して塞ぐ。
func TestDeleteIsBlockedWhenAnyTargetIsBusy(t *testing.T) {
	t.Parallel()

	st := state(t, pagetest.SampleRunner(), pagetest.BusyRunner())
	f := fakeOf(t, st)
	m := newModel(t, st)

	m = send(t, m, page.SetupRequestMsg{
		Op:      page.SetupRemove,
		Runners: []runner.Runner{pagetest.SampleRunner(), pagetest.BusyRunner()},
	})

	if strings.Contains(view(m), "削除の確認") {
		t.Error("1 台でもジョブ実行中なら確認へ進まないこと")
	}
	if len(f.Calls()) != 0 {
		t.Errorf("コマンドを発行している: %v", f.Calls())
	}
}

func TestLifecycleMsgsAreNotEatenByModal(t *testing.T) {
	t.Parallel()

	st := state(t, pagetest.SampleRunner())
	m := newModel(t, st)
	m = send(t, m, page.SetupRequestMsg{
		Op: page.SetupRemove, Runners: []runner.Runner{pagetest.SampleRunner()},
	})

	// モーダルを開いたままでも、後始末の Msg で落ちない。
	m = send(t, m, page.DeactivateMsg{}, page.ActivateMsg{}, page.ShutdownMsg{})
	if view(m) == "" {
		t.Error("後始末の後に描画が空になっている")
	}
}

// Setup タブの esc は Runners へ戻る（screens.md の画面遷移）。
//
// 親は esc をタブの移動に使わないので、戻り先が一意に決まるこのタブが自分で
// 移動を要求する。
func TestEscapeReturnsToRunners(t *testing.T) {
	t.Parallel()

	m := newModel(t, state(t, pagetest.SampleRunner()))

	_, cmd := m.Update(pagetest.Press("esc"))

	found := false
	for _, msg := range pagetest.Msgs(cmd) {
		if open, ok := msg.(page.OpenTabMsg); ok && open.Title == page.TabRunners {
			found = true
		}
	}
	if !found {
		t.Error("esc で Runners タブへ戻る要求が出ていない")
	}
}
