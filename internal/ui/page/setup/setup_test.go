package setup_test

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/appconfig"
	"github.com/ousiassllc/gsr-helper/internal/exec"
	"github.com/ousiassllc/gsr-helper/internal/gh"
	"github.com/ousiassllc/gsr-helper/internal/runner"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/pagetest"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/setup"
)

// state は Setup タブ用の共有状態を返す。
func state(t *testing.T, rs ...runner.Runner) page.StateMsg {
	t.Helper()

	st := pagetest.State(100, 30, rs...)
	st.Setup = page.SetupDeps{
		Host:     "build01",
		Defaults: appconfig.Default().Defaults,
		Secrets:  gh.NewSecrets(),
	}
	return st
}

// newModel は最初の共有状態まで流した Model を返す。
func newModel(t *testing.T, st page.StateMsg) tea.Model {
	t.Helper()

	next, cmd := setup.New(0, st).Update(st)
	return pagetest.Advance(next, cmd, pagetest.AdvanceRounds)
}

// send は Msg を配って落ち着くまで進める。
func send(t *testing.T, m tea.Model, msgs ...tea.Msg) tea.Model {
	t.Helper()

	for _, msg := range msgs {
		next, cmd := m.Update(msg)
		m = pagetest.Advance(next, cmd, pagetest.AdvanceRounds)
	}
	return m
}

// view は現在の描画を返す。
func view(m tea.Model) string { return m.View().Content }

// fakeOf は共有状態の Executor をテスト実装として取り出す。
func fakeOf(t *testing.T, st page.StateMsg) *exec.Fake {
	t.Helper()

	f, ok := st.Exec.(*exec.Fake)
	if !ok {
		t.Fatalf("Exec の型 = %T, want *exec.Fake", st.Exec)
	}
	return f
}

func TestMenuListsThreeEntries(t *testing.T) {
	t.Parallel()

	m := newModel(t, state(t, pagetest.SampleRunner()))
	got := view(m)

	for _, want := range []string{
		"台数を指定して一括追加", "1 台ずつ個別に設定して追加", "バージョンを一括更新",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("メニューに %q が無い:\n%s", want, got)
		}
	}
}

func TestMenuIsGatedByCaps(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		mutate func(*appconfig.Caps)
		want   string
	}{
		"非 root": {func(c *appconfig.Caps) { c.Root = false }, "root 権限が必要です"},
		"gh 未認証": {func(c *appconfig.Caps) { c.GitHubToken = false }, "GitHub の認証が必要です"},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			st := state(t, pagetest.SampleRunner())
			tt.mutate(&st.Caps)

			got := view(newModel(t, st))
			if !strings.Contains(got, tt.want) {
				t.Errorf("理由 %q が出ていない:\n%s", tt.want, got)
			}
		})
	}
}

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

// chromeOf は現在の状態行とフッタを取り出す。
func chromeOf(t *testing.T, m tea.Model) page.ChromeMsg {
	t.Helper()

	_, cmd := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	c, ok := pagetest.ChromeOf(cmd)
	if !ok {
		t.Fatal("ChromeMsg が発行されていない")
	}
	return c
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
