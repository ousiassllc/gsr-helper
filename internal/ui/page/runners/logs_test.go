package runners

import (
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/runner"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/action"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/pagetest"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/pagetest/cmdtest"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/runnerdetail"
)

// `l`（ログを開く）の結線を検証する。**フッタに出しているキーが実際に何かを起こす**
// ことを固定する（出したまま結線しないと「押せるが何も起きない」経路になる）。

// 一覧で l を押すと、その runner のログを Logs タブで開くよう親へ求める。
func TestLogsKeyRequestsLogsTab(t *testing.T) {
	st := pagetest.State(80, 16, pagetest.SampleRunner())
	m, _ := step(t, New(0, st), st)

	_, cmd := step(t, m, pagetest.Press("l"))
	got, err := pagetest.OpenTabOf(cmd, cmdtest.CmdTimeout)
	if err != nil {
		t.Fatalf("l を押してもタブ移動を求めていない: %v", err)
	}
	if got.Title != page.TabLogs {
		t.Errorf("移動先 = %q, want %q", got.Title, page.TabLogs)
	}
	show, ok := got.Msg.(page.ShowLogMsg)
	if !ok {
		t.Fatalf("用件 = %T, want page.ShowLogMsg", got.Msg)
	}
	if show.Runner.Name() != pagetest.SampleRunner().Name() {
		t.Errorf("対象 = %q, want %q", show.Runner.Name(), pagetest.SampleRunner().Name())
	}
}

// 詳細画面から選んだ場合も同じ要求になり、詳細画面は閉じる
// （screens.md の設計原則 6「操作の起点は複数、確認は 1 つ」）。
func TestLogsFromDetailClosesModalAndRequestsTab(t *testing.T) {
	st := pagetest.State(80, 16, pagetest.SampleRunner())
	m, _ := step(t, New(0, st), st)

	m, _ = step(t, m, pagetest.Press("enter"))
	if !m.overlay.Active() {
		t.Fatal("詳細画面が開いていない（前提が崩れている）")
	}

	res := page.ResultMsg{
		Kind: runnerdetail.Kind,
		Msg:  runnerdetail.ChosenMsg{Action: action.Logs, Runner: pagetest.SampleRunner()},
	}
	next, cmd := step(t, m, res)
	if next.overlay.Active() {
		t.Error("ログを開いても詳細画面が開いたままである")
	}
	if _, err := pagetest.OpenTabOf(cmd, cmdtest.CmdTimeout); err != nil {
		t.Errorf("詳細画面からの決定でタブ移動を求めていない: %v", err)
	}
}

// 孤児ユニットの行では何も起きない（対応する runner ディレクトリが無く `_diag` を
// 持たない。openDetail と同じ扱い）。
func TestLogsKeyIgnoresOrphanRow(t *testing.T) {
	st := pagetest.State(80, 16)
	st.Result.OrphanUnits = []runner.SvcState{{
		Unit: "actions.runner.foo-bar.old01.service", Load: "loaded", Active: "failed",
		Sub: "failed", FileState: "enabled", WorkingDir: "", User: "", MainPID: 0,
	}}
	m, _ := step(t, New(0, st), st)

	_, cmd := step(t, m, pagetest.Press("l"))
	if _, err := pagetest.OpenTabOf(cmd, cmdtest.CmdTimeout); err == nil {
		t.Error("孤児ユニットの行でログを開こうとしている")
	}
}
