package runners_test

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/appconfig"
	"github.com/ousiassllc/gsr-helper/internal/runner"
	"github.com/ousiassllc/gsr-helper/internal/runner/scope"
	"github.com/ousiassllc/gsr-helper/internal/ui/keymap"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/runners"
	"github.com/ousiassllc/gsr-helper/internal/ui/token"
)

// press はキー入力の Msg を作る。文字キーは Text、特殊キーは Code で表す。
func press(k string) tea.KeyPressMsg {
	special := map[string]rune{"space": tea.KeySpace, "enter": tea.KeyEnter, "esc": tea.KeyEscape}
	if code, ok := special[k]; ok {
		return tea.KeyPressMsg{Code: code}
	}
	return tea.KeyPressMsg{Text: k, Code: []rune(k)[0]}
}

// testResult は runner 2 台と孤児ユニット 1 件の検出結果を返す。
func testResult() runner.Result {
	return runner.Result{
		Runners: []runner.Runner{sampleRunner("build01-1", false), sampleRunner("build01-2", true)},
		OrphanUnits: []runner.SvcState{{
			Unit: "actions.runner.foo-bar.old01.service", Load: "loaded", Active: "failed",
			Sub: "failed", FileState: "enabled", WorkingDir: "", User: "", MainPID: 0,
		}},
		Warnings: nil,
	}
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

// testState は共有状態のスナップショットを返す。
func testState(w, h int) page.StateMsg {
	return page.StateMsg{
		Result: testResult(),
		Caps: appconfig.Caps{
			Root: true, Systemd: true, Docker: true, Journal: true,
			GitHubToken: true, SudoUser: "ousiass",
		},
		Styles: token.NewStyles(true, false),
		Keys:   keymap.New(),
		Dark:   true,
		BodyW:  w,
		BodyH:  h,
		Err:    nil,
	}
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

// chrome は Cmd から ChromeMsg を取り出す。Batch は展開する。
//
// 見つかった時点で打ち切るのは、絞り込みのカーソル点滅の Cmd（1 秒待つ）を
// 実行しないためである。page は ChromeMsg を Batch の先頭に置いている。
func chrome(t *testing.T, cmd tea.Cmd) page.ChromeMsg {
	t.Helper()

	if c, ok := findChrome(cmd); ok {
		return c
	}
	t.Fatal("ChromeMsg が発行されていない")
	return page.ChromeMsg{}
}

// findChrome は Cmd を辿って最初の ChromeMsg を返す。
func findChrome(cmd tea.Cmd) (page.ChromeMsg, bool) {
	if cmd == nil {
		return page.ChromeMsg{}, false
	}
	switch msg := cmd().(type) {
	case page.ChromeMsg:
		return msg, true
	case tea.BatchMsg:
		for _, c := range msg {
			if v, ok := findChrome(c); ok {
				return v, true
			}
		}
	}
	return page.ChromeMsg{}, false
}

// collect は Cmd が返す Msg を平坦化して返す。
func collect(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		return []tea.Msg{msg}
	}

	var out []tea.Msg
	for _, c := range batch {
		out = append(out, collect(c)...)
	}
	return out
}
