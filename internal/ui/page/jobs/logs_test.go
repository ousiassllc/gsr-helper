package jobs

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/ui/page"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/action"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/pagetest"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/runnerdetail"
)

// `l`（ログを開く）の結線を検証する。Jobs タブのフッタにも `l:ログ` が出るため、
// Runners タブと同じ結線を持たなければ「押せるが何も起きない」経路になる。

// openTabMsg は Cmd に含まれるタブ移動の要求を返す。
func openTabMsg(t *testing.T, cmd tea.Cmd) (page.OpenTabMsg, bool) {
	t.Helper()

	for _, msg := range pagetest.Msgs(cmd) {
		if got, ok := msg.(page.OpenTabMsg); ok {
			return got, true
		}
	}
	return page.OpenTabMsg{}, false
}

// ジョブの行で l を押すと、そのジョブを実行している runner のログを開く（FR-47）。
func TestLogsKeyRequestsLogsTab(t *testing.T) {
	st := pagetest.State(80, 16, pagetest.BusyRunner())
	m, _ := step(t, New(1, st), st)

	_, cmd := step(t, m, pagetest.Press("l"))
	got, ok := openTabMsg(t, cmd)
	if !ok {
		t.Fatal("l を押してもタブ移動を求めていない")
	}
	if got.Title != page.TabLogs {
		t.Errorf("移動先 = %q, want %q", got.Title, page.TabLogs)
	}
	show, ok := got.Msg.(page.ShowLogMsg)
	if !ok {
		t.Fatalf("用件 = %T, want page.ShowLogMsg", got.Msg)
	}
	if show.Runner.Name() != pagetest.BusyRunner().Name() {
		t.Errorf("対象 = %q, want %q", show.Runner.Name(), pagetest.BusyRunner().Name())
	}
}

// 詳細画面から選んだ場合も同じ要求になり、詳細画面は閉じる。
func TestLogsFromDetailClosesModalAndRequestsTab(t *testing.T) {
	st := pagetest.State(80, 16, pagetest.BusyRunner())
	m, _ := step(t, New(1, st), st)

	m, _ = step(t, m, pagetest.Press("enter"))
	if !m.overlay.Active() {
		t.Fatal("詳細画面が開いていない（前提が崩れている）")
	}

	res := page.ResultMsg{
		Kind: runnerdetail.Kind,
		Msg:  runnerdetail.ChosenMsg{Action: action.Logs, Runner: pagetest.BusyRunner()},
	}
	next, cmd := step(t, m, res)
	if next.overlay.Active() {
		t.Error("ログを開いても詳細画面が開いたままである")
	}
	if _, ok := openTabMsg(t, cmd); !ok {
		t.Error("詳細画面からの決定でタブ移動を求めていない")
	}
}

// 行が 1 つも無いときは何も起きない（対象が決まらない）。
func TestLogsKeyWithoutRow(t *testing.T) {
	st := pagetest.State(80, 16, pagetest.SampleRunner())
	m, _ := step(t, New(1, st), st)

	_, cmd := step(t, m, pagetest.Press("l"))
	if _, ok := openTabMsg(t, cmd); ok {
		t.Error("実行中のジョブが無いのにログを開こうとしている")
	}
}
