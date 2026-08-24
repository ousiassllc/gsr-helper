package ui

import (
	"context"
	"errors"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/appconfig"
	"github.com/ousiassllc/gsr-helper/internal/doctor"
	"github.com/ousiassllc/gsr-helper/internal/exec"
	"github.com/ousiassllc/gsr-helper/internal/gh"
	"github.com/ousiassllc/gsr-helper/internal/runner"
	"github.com/ousiassllc/gsr-helper/internal/ui/chrome"
	"github.com/ousiassllc/gsr-helper/internal/ui/discovery"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/pagetest"
)

// 内部テスト（package ui）にしてある。タブのメタ情報・検出の状態・chrome が非公開で、
// タブを差し替えて共有状態の配布を見るには内側へ触る必要があるためである。
//
// 共通の道具は page/pagetest から取る（書き写すと前提が食い違い、行数も増える）。

// App の非公開な状態に触れない道具は page/pagetest にある（pagetest/parent.go）。
// ここでは型引数を App に固定して束縛するだけにする——関数値なので呼び出し側の
// 書き方は移す前と変わらない。**ここへ本体を書き戻さないこと**（`ui` 直下の行数は
// 上限に張り付いている。atomic-design.md のディレクトリの行数）。
var (
	press        = pagetest.Press
	update       = pagetest.Update[App]
	sendKey      = pagetest.SendKey[App]
	press1       = pagetest.Press1[App]
	applyChrome  = pagetest.ApplyChrome[App]
	isQuit       = pagetest.IsQuit
	blocked      = pagetest.Blocked
	sampleRunner = pagetest.SampleRunner
	discovered   = pagetest.Discovered[App]
	takeHostReq  = pagetest.TakeHostReq[App]
)

// expand は Cmd の束を 1 段展開する。締め切り内に戻らなければテストを止める。
//
// **ここで止めるのが要点である。** 素で走らせていたころは戻らない Cmd を渡すと
// パッケージごとハングし、失敗として読めなかった（Issue #145）。
func expand(t *testing.T, cmd tea.Cmd) []tea.Cmd {
	t.Helper()

	cmds, err := pagetest.Expand(cmd, pagetest.CmdTimeout)
	if err != nil {
		t.Fatalf("Cmd の束を展開できない: %v", err)
	}
	return cmds
}

// newApp は親 Model を組み立てる。走査ルートを空にして検出の入力を最小にする。
func newApp(ex exec.Executor) App {
	a := New(appconfig.Default(), pagetest.Caps(), ex, Options{
		Color:   false,
		Refresh: 2 * time.Second,
		Roots:   nil,
		Host:    "build01",
	})
	// **起動時の前提チェック（FR-44）は既定で走らせない。** 本物の項目は実ホストの
	// sudo / docker / /etc/group を読むため、親 Model の検証が実行環境の構成で
	// 揺れる。FR-44 そのものを見るテストは withHostChecks で差し替える。
	a.bg.HostReq.Checks = nil
	// **保有スコープも本物の GitHub へ出させない。** 束の Cmd をすべて実行する検証
	// （applyChrome）があるため、塞がないと api.github.com を叩いて Budget ぶん止まる。
	a.bg.Scopes.NewClient = func(context.Context) (*gh.Client, error) {
		return nil, errors.New("テストでは GitHub へ出ない")
	}
	return a
}

// newAppWithRunner は端末サイズを配り、runner 1 台の検出結果を取り込んだ App を返す。
//
// 既定タブ（Runners）が一覧を持っていることは、キーの閉じ込め（gate_test）とタブを
// またぐ移動（route_test）の前提である。一覧が空だと打鍵が一覧に届かず、閉じ込めも
// 移動も起きないまま緑になる。
func newAppWithRunner(ex exec.Executor) App {
	a, _ := update(newApp(ex), tea.WindowSizeMsg{Width: 100, Height: 30})
	a, _ = update(a, discovery.Msg{
		Seq:    1,
		Result: runner.Result{Runners: []runner.Runner{sampleRunner()}},
		Err:    nil,
	})
	return a
}

// withHostChecks は起動時の前提チェックを差し替えた App を返す。
func withHostChecks(a App, checks ...doctor.Check) App {
	a.bg.HostReq.Checks = checks
	return a
}

// replaceTabs は有効なタブを mk が返す Model に差し替える。並びはタブの並びと同じ
// （無効なタブは差し替えないので、タブの添字とは一致しない）。
func replaceTabs[M tea.Model](a App, mk func(tab int) M) (App, []M) {
	out := make([]M, 0, len(a.tabs))
	for i := range a.tabs {
		if !a.tabs[i].Enabled {
			continue
		}
		m := mk(i)
		a.tabs[i].Model = m
		out = append(out, m)
	}
	return a, out
}

// withSpies は有効なタブを pagetest.Spy に差し替える。
//
// **差し戻し（Bubble）を立てる。** 親はグローバルキーを page が差し戻してきた
// ときにだけ解釈する（keys.go の配送）。立てないとキーの配送を検証できない。
func withSpies(a App) (App, []*pagetest.Spy) {
	return replaceTabs(a, func(tab int) *pagetest.Spy {
		s := pagetest.NewSpy(tab)
		s.Bubble, s.Chrome = true, pageChrome(tab)
		return s
	})
}

// withStreams は有効なタブを pagetest.StreamPage に差し替える（前面で購読を 1 本
// 張り、裏へ回ったら畳む page）。
func withStreams(a App) (App, []*pagetest.StreamPage) {
	return replaceTabs(a, pagetest.NewStreamPage)
}

// statusLine は親が描く状態行を返す。
//
// 組み立ては chrome の純粋関数が持ち、親は値を写すだけである（chrome.go）。
func statusLine(a App) string { return chrome.Status(a.chromeView()) }
