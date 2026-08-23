package setup_test

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/appconfig"
	"github.com/ousiassllc/gsr-helper/internal/exec"
	"github.com/ousiassllc/gsr-helper/internal/runner"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/pagetest"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/setup"
)

// state は Setup タブ用の共有状態を返す。
func state(t *testing.T, rs ...runner.Runner) page.StateMsg {
	t.Helper()

	st, _ := stateAPI(t, rs...)
	return st
}

// stateAPI は共有状態と、そこに載せた外部資源の差し替えを返す。
//
// **外部資源は必ず差し替える。** 差し替えないと internal/setup/job が gh.Token へ
// 落ち、周囲の GH_TOKEN で本物の api.github.com へ短命トークンを発行してしまう
// （page.SetupDeps.NewClient の doc。t.Parallel を使うので t.Setenv も使えない）。
// **このパッケージの共有状態は必ずここを通して作ること。**
func stateAPI(t *testing.T, rs ...runner.Runner) (page.StateMsg, *pagetest.SetupAPI) {
	t.Helper()

	api := pagetest.NewSetupAPI(t.Cleanup)
	st := pagetest.State(100, 30, rs...)
	st.Setup = api.Deps("build01", appconfig.Default().Defaults)
	return st, api
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
