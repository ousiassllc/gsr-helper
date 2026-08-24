// Package pagetest は page とタブ、そのタブを束ねる親 Model の実装を検証するための
// 共通の道具を提供する。
//
// テストファイル（_test.go）ではなく通常のパッケージに置くのは、タブ 1 枚ごとに
// パッケージが分かれる（page/<tab>）ため、_test.go に置いた道具を他のパッケージから
// import できないためである。置かないとタブを足す Issue ごとに共有状態の組み立てと
// spy を作り直すことになり、検証の前提がタブごとに食い違う。exec.Fake も同じ理由で
// 通常のパッケージに置いてある。
//
// **親 Model（internal/ui）の検証もここから取る。** 親の検証はタブを差し替えて行う
// ため道具立てが page 側と同じであり、`ui` 直下に置くと行数上限（1 ディレクトリ
// 2000 行）を押し上げるだけになる（atomic-design.md のディレクトリの行数）。
//
// **本番の経路からは import しない。** 通常のパッケージである以上 Go は止められない
// ので、import_test.go の TestNoProductionCodeImportsTestFixtures が各パッケージの本番
// ファイルの import を読んで検査する（Issue #45）。page の内部テスト（package page）
// からも import できない（このパッケージが page を import するため循環になる）ので、
// そちらは自分の helper_test.go を持つ。
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
	if k == "shift+tab" {
		return tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift}
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
// 本体の領域と検出結果だけを引数に取る。配色・キー定義・能力・Executor・プロセス走査は
// 既定（色なし・全能力・exec.Fake・ScanOf）であり、そこを振るテストは戻り値の該当
// フィールドだけを差し替える。
func State(w, h int, runners ...runner.Runner) page.StateMsg {
	return page.StateMsg{
		Result:    runner.Result{Runners: runners},
		Caps:      Caps(),
		Styles:    Styles(),
		Keys:      Keys(),
		Exec:      exec.NewFake(),
		ScanProcs: ScanOf(runners...),
		Dark:      true,
		BodyW:     w,
		BodyH:     h,
		Err:       nil,
	}
}

// ScanOf は渡した runner が持つ Runner.Worker だけを返すプロセス走査を組む。
//
// **共有状態から実ホストの /proc を締め出すためにある。** page.StateMsg.ScanProcs が
// nil だと svc 側は procs.Scan に落ちる（page.StateMsg.ScanProcs の doc）。そうなると
// ドレイン停止（FR-07）の停止条件は「テストを走らせるホストに `/opt/runners/*` の
// worker が居ないこと」になり、居るホストでは待ち時間が無制限である以上、待機が
// 終わらず cmdtest.Advance が待ち時間切れで panic する（Issue #155）。
//
// 表を runner の Workers から組むので、「一覧が busy と出している runner は走査でも
// busy」という一貫した世界になる。Executor を exec.Fake に固定しているのと同じ趣旨で、
// **State を使うテストは既定でホストに依らない**。
func ScanOf(runners ...runner.Runner) func() ([]runner.Process, error) {
	out := make([]runner.Process, 0, len(runners))
	for _, r := range runners {
		out = append(out, r.Workers...)
	}
	return func() ([]runner.Process, error) { return out, nil }
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
