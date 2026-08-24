package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/config/edit"
	"github.com/ousiassllc/gsr-helper/internal/runner"
	"github.com/ousiassllc/gsr-helper/internal/runner/scope"
	"github.com/ousiassllc/gsr-helper/internal/ui/organism/dialog"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/configmodal"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/pagetest"
)

// tabIndex は Config タブの番号（[6]）。
const tabIndex = 5

// newPage は共有状態を配った Config タブを返す。
func newPage(t *testing.T, rs ...runner.Runner) Model {
	t.Helper()

	st := pagetest.State(80, 24, rs...)
	m := New(tabIndex, st)

	next, _ := m.Update(st)
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("Model 以外が返った: %T", next)
	}
	return got
}

// withTempDir は一時ディレクトリに実体を持つ runner を返す。
func withTempDir(t *testing.T, name, env string) runner.Runner {
	t.Helper()

	dir := filepath.Join(t.TempDir(), name)
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatalf("runner ディレクトリの作成に失敗: %v", err)
	}
	if env != "" {
		if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(env), 0o600); err != nil {
			t.Fatalf(".env の作成に失敗: %v", err)
		}
	}

	return runner.Runner{
		Dir:      dir,
		Config:   runner.Config{AgentName: name},
		Scope:    scope.Scope{Kind: scope.Org, Owner: "foo"},
		UnitName: "actions.runner.foo." + name + ".service",
	}
}

// send は Msg を配り、Model を返す。
func send(t *testing.T, m Model, msg tea.Msg) (Model, tea.Cmd) {
	t.Helper()

	next, cmd := m.Update(msg)
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("Model 以外が返った: %T", next)
	}
	return got, cmd
}

// view は本文（モーダルがあればモーダル）を文字列で返す。
func view(m Model) string { return m.View().Content }

// contains は本文に文字列が含まれるかを返す。
func contains(m Model, s string) bool { return strings.Contains(view(m), s) }

// chromeOf は直近の Cmd から ChromeMsg を取り出す。
func chromeOf(t *testing.T, cmd tea.Cmd) page.ChromeMsg {
	t.Helper()

	got, ok := pagetest.ChromeOf(cmd)
	if !ok {
		t.Fatal("ChromeMsg が発行されていない")
	}
	return got
}

// timeout は Cmd の完了を待つ上限。
const timeout = 3 * time.Second

// readFile は path の内容を返す。
func readFile(path string) (string, error) {
	b, err := os.ReadFile(path)
	return string(b), err
}

// openEnvForm は .env のフォームに変更を入れて確定したところまで進める。
//
// huh のフォームを打鍵で埋める代わりに入力先へ直接書くのは、ここで見たいのが
// 「確定してから書き込むまで」の経路だからである。フォームそのものの動きは
// organism/dialog のテストが持つ。
func openEnvForm(t *testing.T, m Model) Model {
	t.Helper()

	m.vals.Kind = edit.KindEnv
	for i, spec := range edit.EnvKeys {
		if spec.Key == "PATH" {
			m.vals.Env[i], m.vals.EnvBefore[i] = "/opt/bin", "/usr/bin"
		}
	}

	m, _ = send(t, m, page.ResultMsg{Kind: configmodal.FormKind, Msg: dialog.FormDoneMsg{Form: nil}})
	if !m.overlay.Active() {
		t.Fatal("差分の承認が開いていない")
	}
	return m
}

// doneOf は Cmd の束から書き込み・反映の結果を取り出す。
//
// 束（tea.Batch）で返るのは chrome の更新と処理本体が同時に流れるためで、
// 本体だけを取り出して結果を見る。
func doneOf(cmd tea.Cmd) (doneMsg, bool) {
	for _, c := range pagetest.Expand(cmd) {
		msg, ok := pagetest.RunCmd(c, timeout)
		if !ok {
			continue
		}
		if tab, wrapped := msg.(page.TabMsg); wrapped {
			msg = tab.Msg
		}
		if done, is := msg.(doneMsg); is {
			return done, true
		}
	}
	return doneMsg{text: "", err: nil}, false
}
