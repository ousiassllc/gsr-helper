package page

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/appconfig"
	"github.com/ousiassllc/gsr-helper/internal/runner"
	"github.com/ousiassllc/gsr-helper/internal/runner/scope"
	"github.com/ousiassllc/gsr-helper/internal/ui/atom"
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

// stubModal は登録したモーダルが受け取った Msg を記録するテスト用の中身。
//
// 重なりの規則を種類に依らず検証するために使う（具体的なモーダルを混ぜると、
// 検証しているのが規則なのか中身なのか分からなくなる）。
type stubModal struct {
	body   string
	keys   []string
	states int
	size   SizeMsg
}

// tea.Model を実装していることをコンパイル時に確かめる。
var _ tea.Model = (*stubModal)(nil)

// newStub は本文を持つモーダルを組み立てる。
func newStub(body string) Modal {
	return Modal{
		Model: &stubModal{body: body, keys: nil, states: 0, size: SizeMsg{W: 0, H: 0}},
		Title: func(tea.Model) string { return body },
		Hints: func(tea.Model) []atom.Hint {
			return []atom.Hint{{Key: "y", Desc: "実行", Enabled: true, Reason: ""}}
		},
	}
}

func (m *stubModal) Init() tea.Cmd { return nil }

func (m *stubModal) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		m.keys = append(m.keys, msg.String())
	case StateMsg:
		m.states++
	case SizeMsg:
		m.size = msg
	}
	return m, nil
}

func (m *stubModal) View() tea.View { return tea.NewView(m.body) }

// stubOf は登録した stubModal を取り出す。
func stubOf(t *testing.T, o Overlay, kind ModalKind) *stubModal {
	t.Helper()

	m, ok := o.Modal(kind)
	if !ok {
		t.Fatalf("%q が登録されていない", kind)
	}
	stub, ok := m.Model.(*stubModal)
	if !ok {
		t.Fatalf("%q の Model = %T, want *stubModal", kind, m.Model)
	}
	return stub
}
