package logs

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	dlogs "github.com/ousiassllc/gsr-helper/internal/logs"
	"github.com/ousiassllc/gsr-helper/internal/runner"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/pagetest"
)

// テストの道具を集める。共有状態の組み立ては pagetest から取り、ここでは Logs タブに
// 固有のもの（`_diag` を持つ runner、購読を辿る Cmd の実行）だけを持つ。

// testTab は検証で使うタブ番号（screens.md の [4]Logs は添字 3）。
const testTab = 3

// cmdTimeout は Cmd 1 本を待つ上限。
//
// 購読の待ち受け（wait）は行が届くまで戻らない。**戻らないこと自体は正しい**ので、
// 諦めて次へ進むための時間である。行を待つ検証は pumpUntil の条件で止める。
const cmdTimeout = 2 * time.Second

// press はキー入力の Msg を作る（組み立ては pagetest.Press に任せる）。
func press(k string) tea.KeyPressMsg { return pagetest.Press(k) }

// testRunner は dir を `_diag` の親に持つ runner を返す。
func testRunner(name, dir string) runner.Runner {
	r := pagetest.SampleRunner()
	r.Dir = dir
	r.Config.AgentName = name
	r.WorkDir = filepath.Join(dir, "_work")
	r.UnitName = "actions.runner.foo." + name + ".service"
	return r
}

// writeLog は runner の `_diag` にログを 1 つ作り、更新時刻を設定する。
func writeLog(t *testing.T, r runner.Runner, name, body string, mod time.Time) string {
	t.Helper()

	diag := filepath.Join(r.Dir, dlogs.DiagDir)
	if err := os.MkdirAll(diag, 0o755); err != nil {
		t.Fatalf("_diag を作れない: %v", err)
	}
	path := filepath.Join(diag, name)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("%s を書けない: %v", name, err)
	}
	if err := os.Chtimes(path, mod, mod); err != nil {
		t.Fatalf("%s の時刻を設定できない: %v", name, err)
	}
	return path
}

// newTab は最初の共有状態を配り終えた Logs タブを返す。
//
// New の直後ではなく StateMsg を 1 度渡した状態から始めるのは、親が必ずそうする
// ためである（登録の Cmd はここで流れる。runners.go の Init の doc）。
func newTab(t *testing.T, st page.StateMsg) Model {
	t.Helper()

	m, _ := step(t, New(testTab, st), st)
	return m
}

// step は Msg を 1 つ渡し、Model と Cmd を返す。
func step(t *testing.T, m Model, msg tea.Msg) (Model, tea.Cmd) {
	t.Helper()

	next, cmd := m.Update(msg)
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("Update が %T を返した, want logs.Model", next)
	}
	return got, cmd
}

// runCmd は Cmd を 1 本実行して Msg を返す。制限時間内に戻らなければ偽を返す。
//
// チャネルに余裕を持たせるのは、諦めたあとに Cmd が戻ってきても送信で詰まらせない
// ためである（購読を畳めば必ず戻る）。
func runCmd(cmd tea.Cmd) (tea.Msg, bool) {
	if cmd == nil {
		return nil, false
	}
	ch := make(chan tea.Msg, 1)
	go func() { ch <- cmd() }()
	select {
	case msg := <-ch:
		return msg, true
	case <-time.After(cmdTimeout):
		return nil, false
	}
}

// pumpUntil は Cmd を辿って Model を進め、cond が満たされた時点で止める。
//
// bubbletea のランタイムの代わりである。Cmd の並び（tea.Batch）は中身へ辿り、
// page.Do の包み（page.TabMsg）は解いてから Model へ渡す。
func pumpUntil(t *testing.T, m Model, cmd tea.Cmd, cond func(Model) bool) Model {
	t.Helper()

	const maxSteps = 200
	queue := []tea.Cmd{cmd}
	for range maxSteps {
		if cond(m) {
			return m
		}
		if len(queue) == 0 {
			t.Fatalf("Cmd を辿り切ったが条件を満たさなかった")
		}
		next := queue[0]
		queue = queue[1:]

		msg, ok := runCmd(next)
		if !ok {
			continue
		}
		if inner, ok := pagetest.Cmds(msg); ok {
			queue = append(queue, inner...)
			continue
		}
		if tm, ok := msg.(page.TabMsg); ok {
			if tm.Tab != testTab {
				t.Fatalf("TabMsg のタブ番号 = %d, want %d", tm.Tab, testTab)
			}
			msg = tm.Msg
		}
		if msg == nil {
			continue
		}
		var c tea.Cmd
		m, c = step(t, m, msg)
		queue = append(queue, c)
	}
	t.Fatalf("%d 手進めても条件を満たさなかった", maxSteps)
	return m
}

// chromeOf は Cmd に含まれる ChromeMsg を返す。
func chromeOf(t *testing.T, cmd tea.Cmd) page.ChromeMsg {
	t.Helper()

	for _, c := range pagetest.Expand(cmd) {
		if c == nil {
			continue
		}
		if msg, ok := c().(page.ChromeMsg); ok {
			return msg
		}
	}
	t.Fatal("ChromeMsg が返っていない")
	return page.ChromeMsg{}
}
