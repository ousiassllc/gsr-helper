package page

import (
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/appconfig"
	"github.com/ousiassllc/gsr-helper/internal/runner"
	"github.com/ousiassllc/gsr-helper/internal/runner/scope"
	"github.com/ousiassllc/gsr-helper/internal/ui/keymap"
	"github.com/ousiassllc/gsr-helper/internal/ui/token"
)

// テストは内部テスト（package page）にしてある。詳細画面が組み立てた操作リストへ
// 直接キーを届けるといった、実装の内側の経路を検証するためである。

// press はキー入力の Msg を作る。文字キーは Text、特殊キーは Code で表す
// （bubbletea v2 の Key.String は Text があればそれを、無ければ keystroke を返す）。
func press(k string) tea.KeyPressMsg {
	special := map[string]rune{
		"space": tea.KeySpace, "enter": tea.KeyEnter, "esc": tea.KeyEscape, "tab": tea.KeyTab,
		"up": tea.KeyUp, "down": tea.KeyDown,
	}
	if code, ok := special[k]; ok {
		return tea.KeyPressMsg{Code: code}
	}
	if rest, ok := strings.CutPrefix(k, "ctrl+"); ok {
		return tea.KeyPressMsg{Code: rune(rest[0]), Mod: tea.ModCtrl}
	}
	return tea.KeyPressMsg{Text: k, Code: []rune(k)[0]}
}

// testStyles は色を使わないスタイル。期待文字列に ANSI 列が混ざらないようにする。
func testStyles() token.Styles {
	return token.NewStyles(true, false)
}

// fullCaps はすべての能力がある状態。
func fullCaps() appconfig.Caps {
	return appconfig.Caps{
		Root:        true,
		Systemd:     true,
		Docker:      true,
		Journal:     true,
		GitHubToken: true,
		SudoUser:    "ousiass",
	}
}

// sampleRunner は systemd 管理で稼働中の runner を返す。
func sampleRunner() runner.Runner {
	unit := "actions.runner.foo.build01-1.service"
	return runner.Runner{
		Dir: "/opt/runners/build01-1",
		Config: runner.Config{
			AgentID: 1, AgentName: "build01-1", PoolID: 0, PoolName: "",
			ServerURL: "", GitHubURL: "https://github.com/orgs/foo",
			WorkFolder: "_work", Ephemeral: false, DisableUpdate: true,
		},
		Scope:     scope.Scope{Kind: scope.Org, Owner: "foo", Repo: ""},
		Version:   "2.309.0",
		WorkDir:   "/opt/runners/build01-1/_work",
		UnitName:  unit,
		RunAsUser: "runner",
		Managed:   runner.ManagedSystemd,
		Svc: &runner.SvcState{
			Unit: unit, Load: "loaded", Active: "active", Sub: "running",
			FileState: "enabled", WorkingDir: "/opt/runners/build01-1",
			User: "runner", MainPID: 284102,
		},
		Listener: &runner.Process{
			PID: 284102, Kind: runner.ProcListener, Dir: "/opt/runners/build01-1",
			Started: time.Now().Add(-time.Hour), Exe: "", UID: 1000,
		},
		Workers: nil,
	}
}

// busyRunner はジョブを実行中の runner を返す。
func busyRunner() runner.Runner {
	r := sampleRunner()
	r.Workers = []runner.Process{{
		PID: 284193, Kind: runner.ProcWorker, Dir: r.Dir,
		Started: time.Now().Add(-4 * time.Minute), Exe: "", UID: 1000,
	}}
	return r
}

// standaloneRunner は run.sh を直起動している runner を返す。
func standaloneRunner() runner.Runner {
	r := sampleRunner()
	r.UnitName = ""
	r.Svc = nil
	r.Managed = runner.ManagedStandalone
	return r
}

// testKeys はキー定義の集約を返す。
func testKeys() keymap.Set { return keymap.New() }
