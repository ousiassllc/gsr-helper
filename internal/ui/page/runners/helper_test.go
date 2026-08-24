package runners_test

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/runner"
	"github.com/ousiassllc/gsr-helper/internal/runner/scope"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/pagetest"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/pagetest/cmdtest"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/runners"
)

// press はキー入力の Msg を作る。組み立ては pagetest.Press に任せる
// （タブごとに写すと、特殊キーの表し方が食い違って実端末が送らない Msg で検証して
// しまう。Issue #31 の jobs 側がその状態だった）。
func press(k string) tea.KeyPressMsg { return pagetest.Press(k) }

// orphanUnits は孤児ユニット 1 件（サービスは残っているがディレクトリが無い）を返す。
// pagetest の共有状態は孤児を持たないので、この場で足す。
func orphanUnits() []runner.SvcState {
	return []runner.SvcState{{
		Unit: "actions.runner.foo-bar.old01.service", Load: "loaded", Active: "failed",
		Sub: "failed", FileState: "enabled", WorkingDir: "", User: "", MainPID: 0,
	}}
}

// sampleRunner は systemd 管理の runner を返す。busy が真ならジョブを実行中にする。
func sampleRunner(name string, busy bool) runner.Runner {
	dir := "/opt/runners/" + name
	unit := "actions.runner.foo." + name + ".service"
	r := runner.Runner{
		Dir:       dir,
		Config:    runner.Config{AgentName: name, GitHubURL: "https://github.com/orgs/foo", WorkFolder: "_work"},
		Scope:     scope.Scope{Kind: scope.Org, Owner: "foo"},
		Version:   "2.311.0",
		WorkDir:   dir + "/_work",
		UnitName:  unit,
		RunAsUser: "runner",
		Managed:   runner.ManagedSystemd,
		Svc: &runner.SvcState{
			Unit: unit, Load: "loaded", Active: "active", Sub: "running",
			FileState: "enabled", WorkingDir: dir, User: "runner", MainPID: 100,
		},
		Listener: &runner.Process{PID: 100, Kind: runner.ProcListener, Dir: dir, UID: 1000},
		Workers:  nil,
	}
	if busy {
		r.Workers = []runner.Process{{
			PID: 284193, Kind: runner.ProcWorker, Dir: dir,
			Started: time.Now().Add(-4 * time.Minute), UID: 1000,
		}}
	}
	return r
}

// unmanagedRunner は未稼働でサービス登録もされていない runner
// （runner.ManagedUnknown）を返す。ユニット名・Listener・Worker のどれも持たないため、
// サービス制御が発行できるコマンドは 1 本も無い。
func unmanagedRunner(name string) runner.Runner {
	r := sampleRunner(name, false)
	r.UnitName = ""
	r.Managed = runner.ManagedUnknown
	r.Svc = nil
	r.Listener = nil
	r.Workers = nil
	return r
}

// testState は共有状態のスナップショットを返す。
//
// **私物の組み立てを持たない。** 自前で組んでいた頃は Exec を nil のままにしており、
// 「systemctl が無い環境でも nil にはしない」という page.StateMsg の不変条件に反する、
// **親が決して作らない状態**でしか Runners タブを検証していなかった（Issue #31）。
func testState(w, h int) page.StateMsg {
	st := pagetest.State(w, h, sampleRunner("build01-1", false), sampleRunner("build01-2", true))
	st.Result.OrphanUnits = orphanUnits()
	return st
}

// newModel は共有状態を配った状態の Runners タブを返す。
func newModel(t *testing.T, w, h int) (tea.Model, page.ChromeMsg) {
	t.Helper()

	m, cmd := runners.New(0, testState(w, h)).Update(testState(w, h))
	return m, chrome(t, cmd)
}

// send はキーを順に送り、最後の ChromeMsg を返す。
func send(t *testing.T, m tea.Model, keys ...string) (tea.Model, page.ChromeMsg) {
	t.Helper()

	var c page.ChromeMsg
	for _, k := range keys {
		var cmd tea.Cmd
		m, cmd = m.Update(press(k))
		c = chrome(t, cmd)
	}
	return m, c
}

// chrome は Cmd から ChromeMsg を取り出す。
//
// **走査は page/pagetest に任せる。** 以前はこのファイルが同じ再帰
// （`findChrome`）と Msg の平坦化（`collect`）を持っていたが、どちらも
// `cmdtest.ChromeOf` / `cmdtest.Msgs` の写しであり、道具はタブごとに写さず
// 共有の置き場から取るのが本書の方針である（helper_test.go 冒頭の方針。Issue #107）。
//
// `ChromeOf` が最初の 1 件で打ち切るので、絞り込みのカーソル点滅の Cmd（1 秒待つ）は
// 実行されない。page は ChromeMsg を束の先頭に置いている。
func chrome(t *testing.T, cmd tea.Cmd) page.ChromeMsg {
	t.Helper()

	c, err := cmdtest.ChromeOf(cmd, cmdtest.CmdTimeout)
	if err != nil {
		t.Fatalf("ChromeMsg を取り出せない: %v", err)
	}
	return c
}
