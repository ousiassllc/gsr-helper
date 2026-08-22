// Package pagetest は page とタブの実装を検証するための共通の道具を提供する。
//
// テストファイル（_test.go）ではなく通常のパッケージに置くのは、タブ 1 枚ごとに
// パッケージが分かれる（page/<tab>）ため、_test.go に置いた道具を他のパッケージから
// import できないためである。置かないとタブを足す Issue ごとに共有状態の組み立てと
// spy を作り直すことになり、検証の前提がタブごとに食い違う。exec.Fake も同じ理由で
// 通常のパッケージに置いてある。
//
// 本番の経路からは import しない。page の内部テスト（package page）からも import
// できない（このパッケージが page を import するため循環になる）ので、そちらは
// 自分の helper_test.go を持つ。
package pagetest

import (
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/appconfig"
	"github.com/ousiassllc/gsr-helper/internal/exec"
	"github.com/ousiassllc/gsr-helper/internal/runner"
	"github.com/ousiassllc/gsr-helper/internal/runner/scope"
	"github.com/ousiassllc/gsr-helper/internal/ui/keymap"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
	"github.com/ousiassllc/gsr-helper/internal/ui/token"
)

// Press はキー入力の Msg を作る。文字キーは Text、特殊キーは Code で表す
// （bubbletea v2 の Key.String は Text があればそれを、無ければ keystroke を返す）。
func Press(k string) tea.KeyPressMsg {
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

// Styles は色を使わないスタイルを返す。期待文字列に ANSI 列が混ざらないようにする。
func Styles() token.Styles { return token.NewStyles(true, false) }

// Keys はキー定義の集約を返す。
func Keys() keymap.Set { return keymap.New() }

// Caps はすべての能力がある状態を返す。
func Caps() appconfig.Caps {
	return appconfig.Caps{
		Root:        true,
		Systemd:     true,
		Docker:      true,
		Journal:     true,
		GitHubToken: true,
		SudoUser:    "ousiass",
	}
}

// State は共有状態のスナップショットを組み立てる。
//
// 本体の領域と検出結果だけを引数に取る。配色・キー定義・能力・Executor は既定
// （色なし・全能力・exec.Fake）であり、そこを振るテストは戻り値の該当フィールドだけを
// 差し替える。
func State(w, h int, runners ...runner.Runner) page.StateMsg {
	return page.StateMsg{
		Result: runner.Result{Runners: runners},
		Caps:   Caps(),
		Styles: Styles(),
		Keys:   Keys(),
		Exec:   exec.NewFake(),
		Dark:   true,
		BodyW:  w,
		BodyH:  h,
		Err:    nil,
	}
}

// SampleRunner は systemd 管理で稼働中の runner を返す。
func SampleRunner() runner.Runner {
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

// BusyRunner はジョブを実行中の runner を返す。Dir は SampleRunner と同じであり、
// 「同じ runner の状態が変わった」周期を作れる。
func BusyRunner() runner.Runner {
	r := SampleRunner()
	r.Workers = []runner.Process{{
		PID: 284193, Kind: runner.ProcWorker, Dir: r.Dir,
		Started: time.Now().Add(-4 * time.Minute), Exe: "", UID: 1000,
	}}
	return r
}

// StandaloneRunner は run.sh を直起動している runner を返す。
func StandaloneRunner() runner.Runner {
	r := SampleRunner()
	r.UnitName = ""
	r.Svc = nil
	r.Managed = runner.ManagedStandalone
	return r
}

// UnavailableRunner は systemd の状態が判定できなかった runner を返す
// （systemctl list-units 自体が失敗した場合。runner.ManagedUnavailable）。
func UnavailableRunner() runner.Runner {
	r := StandaloneRunner()
	r.Listener = nil
	r.Managed = runner.ManagedUnavailable
	return r
}
