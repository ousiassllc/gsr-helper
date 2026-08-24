package setup_test

import (
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/runner"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/pagetest"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/pagetest/cmdtest"
)

// 削除は承認の前に必ず確認を挟み、そこで実行するコマンド全文を出す（FR-16）。
// キャンセルすれば 1 本も発行せずに閉じる。
//
// 「確認が出ること」「何が書いてあるか」「やめれば走らないこと」は同じ 1 本の
// 流れでしか確かめられないので、まとめて見る。トークンは計画の時点から *** で
// ある（confirm.go の confirmInput の doc）。
func TestDeleteConfirmShowsPlannedCommandsBeforeAnyCommand(t *testing.T) {
	t.Parallel()

	st := state(t, pagetest.SampleRunner())
	f := fakeOf(t, st)
	m := pagetest.Quick(newModel(t, st), pagetest.RemoveRequest(pagetest.SampleRunner()))

	got := view(m)
	if !strings.Contains(got, "削除の確認") {
		t.Fatalf("確認ダイアログが出ていない:\n%s", got)
	}
	for _, want := range []string{
		"./svc.sh stop", "./svc.sh uninstall", "./config.sh remove --token ***",
		// FR-18: runner ディレクトリは残す。残る場所を示す。
		"runner ディレクトリは削除されません",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("確認に %q が出ていない:\n%s", want, got)
		}
	}
	if len(f.Calls()) != 0 {
		t.Errorf("承認の前にコマンドを発行している: %v", f.Calls())
	}

	if m = pagetest.Quick(m, pagetest.Press("n")); strings.Contains(view(m), "削除の確認") {
		t.Error("キャンセルしても確認が閉じていない")
	}
	if len(f.Calls()) != 0 {
		t.Errorf("キャンセルしたのにコマンドを発行している: %v", f.Calls())
	}
}

// ジョブ実行中の runner を含む削除は、対象ごとに判定して塞ぐ。
//
// 1 台だけなら状態行でドレイン停止を促す。複数のうち 1 台でも実行中なら全体を
// 塞ぐ——実行中の 1 台を黙って除くと、承認した台数と実際に消える台数が食い違う。
func TestDeleteIsBlockedWhileAnyTargetIsBusy(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		targets []runner.Runner
		hint    bool // 状態行にドレイン停止の案内を求めるか
	}{
		"実行中の 1 台":     {[]runner.Runner{pagetest.BusyRunner()}, true},
		"1 台でも実行中なら全体": {[]runner.Runner{pagetest.SampleRunner(), pagetest.BusyRunner()}, false},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			st := state(t, tt.targets...)
			f := fakeOf(t, st)
			m := pagetest.Quick(newModel(t, st), pagetest.RemoveRequest(tt.targets...))

			if strings.Contains(view(m), "削除の確認") {
				t.Error("ジョブ実行中の runner の削除で確認が開いている")
			}
			if len(f.Calls()) != 0 {
				t.Errorf("コマンドを発行している: %v", f.Calls())
			}
			if s := chromeOf(t, m).Status; tt.hint && !strings.Contains(s, "ドレイン停止") {
				t.Errorf("状態行 = %q, ドレイン停止を促すこと", s)
			}
		})
	}
}

// 承認して初めて実行が始まる。**そのとき外へ出ないことも同時に見張る。**
// 差し替えが外れると job は gh.Token へ落ち、周囲の GH_TOKEN で本物の
// api.github.com へ remove-token を発行する（page.SetupDeps の doc）。
func TestApprovalStartsTheRun(t *testing.T) {
	t.Parallel()

	st, api := pagetest.SetupState(t.Cleanup, pagetest.SampleRunner())
	m := pagetest.Quick(newModel(t, st), pagetest.RemoveRequest(pagetest.SampleRunner()))
	m = pagetest.Quick(m, pagetest.Press("y"))

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
	if api.Clients() == 0 {
		t.Fatal("差し替えた API クライアントが使われていない")
	}
	if !strings.Contains(strings.Join(api.Paths(), " "), "remove-token") {
		t.Errorf("短命トークンの発行が模したサーバへ来ていない: %v", api.Paths())
	}
}

func TestLifecycleMsgsAreNotEatenByModal(t *testing.T) {
	t.Parallel()

	m := pagetest.Quick(newModel(t, state(t, pagetest.SampleRunner())),
		pagetest.RemoveRequest(pagetest.SampleRunner()))

	// モーダルを開いたままでも、後始末の Msg で落ちない。
	m = pagetest.Quick(m, page.DeactivateMsg{}, page.ActivateMsg{}, page.ShutdownMsg{})
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

	if !slices.ContainsFunc(cmdtest.MustMsgs(cmd, cmdtest.CmdTimeout), func(msg tea.Msg) bool {
		open, ok := msg.(page.OpenTabMsg)
		return ok && open.Title == page.TabRunners
	}) {
		t.Error("esc で Runners タブへ戻る要求が出ていない")
	}
}
