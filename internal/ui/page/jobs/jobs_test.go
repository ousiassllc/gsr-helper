package jobs_test

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/ousiassllc/gsr-helper/internal/appconfig"
	"github.com/ousiassllc/gsr-helper/internal/runner"
	"github.com/ousiassllc/gsr-helper/internal/runner/scope"
	"github.com/ousiassllc/gsr-helper/internal/ui/keymap"
	"github.com/ousiassllc/gsr-helper/internal/ui/molecule"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/jobs"
	"github.com/ousiassllc/gsr-helper/internal/ui/token"
)

// press はキー入力の Msg を作る。
func press(k string) tea.KeyPressMsg {
	if k == "enter" {
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	}
	return tea.KeyPressMsg{Text: k, Code: []rune(k)[0]}
}

// busyRunner はジョブを count 件実行している runner を返す。
func busyRunner(name string, count int) runner.Runner {
	dir := "/opt/runners/" + name
	r := runner.Runner{
		Dir:      dir,
		Config:   runner.Config{AgentName: name, GitHubURL: "https://github.com/orgs/foo", WorkFolder: "_work"},
		Scope:    scope.Scope{Kind: scope.Org, Owner: "foo"},
		Version:  "2.311.0",
		WorkDir:  dir + "/_work",
		UnitName: "actions.runner.foo." + name + ".service",
		Managed:  runner.ManagedSystemd,
		Svc:      nil,
		Listener: &runner.Process{PID: 100, Kind: runner.ProcListener, Dir: dir, UID: 1000},
		Workers:  nil,
	}
	for i := range count {
		r.Workers = append(r.Workers, runner.Process{
			PID: 284000 + i, Kind: runner.ProcWorker, Dir: dir,
			Started: time.Now().Add(-4 * time.Minute), UID: 1000,
		})
	}
	return r
}

// testState は共有状態のスナップショットを返す。
func testState(runners ...runner.Runner) page.StateMsg {
	return page.StateMsg{
		Result: runner.Result{Runners: runners, OrphanUnits: nil, Warnings: nil},
		Caps: appconfig.Caps{
			Root: true, Systemd: true, Docker: true, Journal: true,
			GitHubToken: true, SudoUser: "ousiass",
		},
		Styles: token.NewStyles(true, false),
		Keys:   keymap.New(),
		Dark:   true,
		BodyW:  80,
		BodyH:  16,
		Err:    nil,
	}
}

// newModel は共有状態を配った Jobs タブを返す。
func newModel(t *testing.T, runners ...runner.Runner) (tea.Model, page.ChromeMsg) {
	t.Helper()

	st := testState(runners...)
	m, cmd := jobs.New(1, st).Update(st)
	return m, chrome(t, cmd)
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

// 実行中ジョブが無い場合はその旨を表示する（screens.md の Jobs タブ）。
func TestEmptyJobs(t *testing.T) {
	m, c := newModel(t, busyRunner("build01-1", 0))
	if got := m.View().Content; !strings.Contains(got, "実行中のジョブはありません") {
		t.Errorf("表示 = %q, ジョブが無い旨を出していない", got)
	}
	if len(c.Footer) != 0 {
		t.Errorf("フッタ = %+v, 対象が無いので空であるべき", c.Footer)
	}
}

// 複数 runner の Worker が runner 横断で 1 覧に並ぶ。
func TestJobsAcrossRunners(t *testing.T) {
	m, _ := newModel(t, busyRunner("build01-1", 2), busyRunner("build01-7", 1))
	got := m.View().Content
	for _, want := range []string{"build01-1", "build01-7", "284000", "284001"} {
		if !strings.Contains(got, want) {
			t.Errorf("%q が表示に無い", want)
		}
	}

	// リポジトリは /proc から取れないため "-" を出す（ログのパッケージで埋める）。
	if !strings.Contains(got, token.IconNoUnit) {
		t.Error("リポジトリ列に値なしの記号が出ていない")
	}
}

// enter で当該 runner の詳細画面を開く（FR-47）。
func TestEnterOpensRunnerDetail(t *testing.T) {
	m, c := newModel(t, busyRunner("build01-1", 1))
	if c.Modal {
		t.Fatal("初期状態でモーダルが開いている")
	}

	m, cmd := m.Update(press("enter"))
	c = chrome(t, cmd)
	if !c.Modal {
		t.Fatal("enter で詳細画面が開かない")
	}

	got := m.View().Content
	for _, want := range []string{"build01-1", "詳細", "スコープ"} {
		if !strings.Contains(got, want) {
			t.Errorf("詳細画面に %q が無い", want)
		}
	}
}

// フッタは screens.md の Jobs タブの文言をキーごとに出す（FR-47）。
//
// 一律の接頭辞を付けると「l:runner をログを開く」のように日本語として崩れる上に
// 幅 80 に収まらないため、キーごとの文言を仕様側に固定する。操作対象が runner で
// あることは先頭の `enter:runner の詳細` が示す。
func TestFooterUsesSpecWordingPerKey(t *testing.T) {
	_, c := newModel(t, busyRunner("build01-1", 1))

	want := []struct{ key, desc string }{
		{"enter", "runner の詳細"},
		{"d", "ドレイン"},
		{"X", "強制停止"},
		{"R", "再起動"},
		{"l", "ログ"},
	}
	if len(c.Footer) != len(want) {
		t.Fatalf("フッタのヒント = %+v, want %d 件", c.Footer, len(want))
	}
	for i, w := range want {
		if c.Footer[i].Key != w.key || c.Footer[i].Desc != w.desc {
			t.Errorf("%d 番目のヒント = %q/%q, want %q/%q",
				i, c.Footer[i].Key, c.Footer[i].Desc, w.key, w.desc)
		}
		if !c.Footer[i].Enabled && c.Footer[i].Reason == "" {
			t.Errorf("無効なキー %q に理由が無い", c.Footer[i].Key)
		}
	}
}

// 幅 80 のフッタ 1 行目に screens.md の Jobs タブのキーがすべて出る。
//
// 設計原則 1「有効なキーを常に画面に出す」の受け入れ条件であり、Runners タブの
// TestFooterShowsEverySpecKeyAtWidth80 と同じ不変条件である。文言を長くすると
// 末尾のキーが ?:ヘルプ に集約されて画面から消えるため、幅 80 で固定する。
func TestFooterShowsEverySpecKeyAtWidth80(t *testing.T) {
	_, c := newModel(t, busyRunner("build01-1", 1))

	line := strings.Split(molecule.KeyBar(c.Footer, 80, token.NewStyles(true, false)), "\n")[0]
	for _, want := range []string{
		"enter:runner の詳細", "d:ドレイン", "X:強制停止", "R:再起動", "l:ログ", "?:ヘルプ",
	} {
		if !strings.Contains(line, want) {
			t.Errorf("フッタ 1 行目に %q が無い: %q", want, line)
		}
	}
	if w := lipgloss.Width(line); w > 80 {
		t.Errorf("フッタ 1 行目の幅 = %d, want 80 以下（%q）", w, line)
	}
}

// モーダル表示中のキーは背後の一覧へ届かない（Runners タブと同じ不変条件）。
func TestModalKeysDoNotReachList(t *testing.T) {
	m, _ := newModel(t, busyRunner("build01-1", 1), busyRunner("build01-7", 1))
	before := cursorRow(t, m)

	m, cmd := m.Update(press("enter"))
	if c := chrome(t, cmd); !c.Modal {
		t.Fatal("enter で詳細画面が開かない")
	}

	for _, k := range []string{"j", "j", "G"} {
		m, cmd = m.Update(press(k))
		if c := chrome(t, cmd); !c.Modal {
			t.Fatalf("キー %q でモーダルが閉じている", k)
		}
	}

	m, cmd = m.Update(press("esc"))
	if c := chrome(t, cmd); c.Modal {
		t.Fatal("esc でモーダルが閉じない")
	}
	if got := cursorRow(t, m); got != before {
		t.Errorf("閉じた後のカーソル行 = %q, want %q（背後の一覧でカーソルが動いた）", got, before)
	}
}

// cursorRow はカーソル記号が付いている行を返す。
func cursorRow(t *testing.T, m tea.Model) string {
	t.Helper()

	for _, line := range strings.Split(m.View().Content, "\n") {
		if strings.Contains(line, token.IconCursor) {
			return line
		}
	}
	t.Fatal("カーソル行が見つからない")
	return ""
}
