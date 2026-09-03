package setup_test

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/exec"
	"github.com/ousiassllc/gsr-helper/internal/runner"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/pagetest"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/pagetest/cmdtest"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/setup"
)

// 共有の足回りは pagetest へ寄せてある。ここに残すのは setup を import する
// newModel（pagetest からは循環になる）と、t.Fatalf を添える薄い包みだけ。

// state は Setup タブ用の共有状態を返す。外部資源は差し替え済み
// （pagetest.SetupState の doc。差し替えを外すと本物の GitHub を叩く）。
func state(t *testing.T, rs ...runner.Runner) page.StateMsg {
	t.Helper()

	st, _ := pagetest.SetupState(t.Cleanup, rs...)
	return st
}

// newModel は最初の共有状態まで流した Model を返す。
func newModel(t *testing.T, st page.StateMsg) tea.Model {
	t.Helper()

	next, cmd := setup.New(0, st).Update(st)
	return cmdtest.Advance(next, cmd, cmdtest.AdvanceRounds)
}

// view は現在の描画を返す。
func view(m tea.Model) string { return m.View().Content }

// fakeOf は共有状態の Executor をテスト実装として取り出す。
func fakeOf(t *testing.T, st page.StateMsg) *exec.Fake {
	t.Helper()

	f, ok := pagetest.FakeOf(st)
	if !ok {
		t.Fatalf("Exec の型 = %T, want *exec.Fake", st.Exec)
	}
	return f
}

// chromeOf は現在の状態行とフッタを取り出す。
func chromeOf(t *testing.T, m tea.Model) page.ChromeMsg {
	t.Helper()

	c, err := pagetest.ChromeAfter(m, cmdtest.CmdTimeout)
	if err != nil {
		t.Fatalf("ChromeMsg を取り出せない: %v", err)
	}
	return c
}
