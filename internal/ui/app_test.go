package ui

import (
	"errors"
	"image/color"
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/exec"
	"github.com/ousiassllc/gsr-helper/internal/runner"
	"github.com/ousiassllc/gsr-helper/internal/ui/template"
)

// Init は背景色の問い合わせと最初の周期の Tick の 2 本を発行する。
//
// 検出は Init から直に発行しない（実行中の本数を親が数えられないため。Init の doc）。
func TestInitEmitsBackgroundColorAndFirstTick(t *testing.T) {
	fake := exec.NewFake()
	cmds := cmdList(newApp(fake).Init())
	if len(cmds) != 2 {
		t.Fatalf("Init が発行した Cmd の本数 = %d, want 2", len(cmds))
	}

	// 1 本目は背景色の問い合わせ。RequestBackgroundColor は Cmd ではなく Msg を
	// 返す関数なので、関数で包まれていることをここで固定する。
	if got := cmds[0](); !reflect.DeepEqual(got, tea.RequestBackgroundColor()) {
		t.Errorf("1 本目の Msg = %#v, want 背景色の問い合わせ", got)
	}

	// 2 本目は即時の tickMsg。検出はこの Msg を受けた Update が始める。
	if _, ok := cmds[1]().(tickMsg); !ok {
		t.Errorf("2 本目の Msg = %T, want tickMsg", cmds[1]())
	}
	if n := len(fake.Calls()); n != 0 {
		t.Errorf("Init が Executor を使っている（%d 件）", n)
	}
}

// 最初の tickMsg で検出が走り、Executor を使う（systemd がある能力なので
// systemctl を叩く）。
func TestFirstTickRunsDiscover(t *testing.T) {
	fake := exec.NewFake()
	a, cmd := update(newApp(fake), tickMsg{})
	cmds := cmdList(cmd)
	if len(cmds) != 2 {
		t.Fatalf("tickMsg が発行した Cmd の本数 = %d, want 2（検出 + 次の Tick）", len(cmds))
	}
	if _, ok := cmds[0]().(discoveredMsg); !ok {
		t.Errorf("1 本目の Msg = %T, want discoveredMsg", cmds[0]())
	}
	if len(fake.Calls()) == 0 {
		t.Error("検出が Executor を使っていない")
	}
	if a.inflight != 1 {
		t.Errorf("実行中の検出の本数 = %d, want 1", a.inflight)
	}
}

// tickMsg は検出と次の Tick を返す。Update の中でドメイン層を直接呼ばない。
func TestTickEmitsDiscoverAndNextTick(t *testing.T) {
	fake := exec.NewFake()
	_, cmd := update(newApp(fake), tickMsg{})
	if got := len(cmdList(cmd)); got != 2 {
		t.Fatalf("tickMsg で発行された Cmd の本数 = %d, want 2", got)
	}
	if n := len(fake.Calls()); n != 0 {
		t.Errorf("Update の中でドメイン層を呼んでいる（Executor の呼び出し %d 件）", n)
	}
}

// systemctl が無い環境では Executor を渡さず、systemd を参照しない。
func TestDiscoverWithoutSystemd(t *testing.T) {
	fake := exec.NewFake()
	a := newApp(fake)
	a.caps.Systemd = false

	if got := a.discoverExec(); got != nil {
		t.Errorf("Executor = %v, want nil（systemd 不在の縮退）", got)
	}
	a.discover()()
	if n := len(fake.Calls()); n != 0 {
		t.Errorf("systemd が無いのにコマンドを %d 件発行している", n)
	}
}

// discoveredMsg は有効な全タブへ配られる。
func TestDiscoveredDistributesToAllTabs(t *testing.T) {
	a, spies := withSpies(newApp(exec.NewFake()))
	res := runner.Result{
		Runners:     nil,
		OrphanUnits: []runner.SvcState{{Unit: "actions.runner.foo.old.service"}},
		Warnings:    nil,
	}

	a, _ = update(a, discoveredMsg{result: res, err: nil})
	for i, s := range spies {
		if len(s.states) != 1 {
			t.Fatalf("タブ %d が受け取った StateMsg = %d 件, want 1", i, len(s.states))
		}
		if got := len(s.states[0].Result.OrphanUnits); got != 1 {
			t.Errorf("タブ %d に配られた孤児ユニット = %d 件, want 1", i, got)
		}
	}

	// 孤児ユニットの件数は親が状態行に出す。
	if got := a.status(); !strings.Contains(got, "孤児ユニット 1 件") {
		t.Errorf("状態行 = %q, 孤児ユニットの件数が無い", got)
	}
}

// 検出のエラーは状態行に出し、画面遷移は巻き戻さない。
func TestDiscoverErrorGoesToStatus(t *testing.T) {
	a, _ := update(newApp(exec.NewFake()), discoveredMsg{
		result: runner.Result{},
		err:    errTest,
	})
	if got := a.status(); !strings.Contains(got, errTest.Error()) {
		t.Errorf("状態行 = %q, エラーが無い", got)
	}
	if a.active != 0 {
		t.Errorf("エラーでタブが変わっている（active = %d）", a.active)
	}
}

// errTest は検出のエラーを模した値。
var errTest = errors.New("検出が間に合いませんでした")

// 期限切れ・失敗した周期の部分結果で直前の成功結果を上書きしない。
//
// runner.Discover は ctx がキャンセルされた時点で残りの systemctl show を発行せず、
// 取れた分だけを返す。その部分結果を採ると systemd 管理の runner が run.sh / - と
// 誤表示され、⚠ が誤って点き、孤児ユニットも過少報告される。
func TestDiscoverErrorKeepsLastResult(t *testing.T) {
	success := runner.Result{
		Runners:     []runner.Runner{sampleRunner()},
		OrphanUnits: []runner.SvcState{{Unit: "actions.runner.foo.old.service"}},
		Warnings:    nil,
	}

	a, spies := withSpies(newApp(exec.NewFake()))
	a, _ = update(a, discoveredMsg{result: success, err: nil})

	// 期限切れの周期。取れた分だけの部分結果（runner 0 台・孤児 0 件）が届く。
	a, _ = update(a, discoveredMsg{result: runner.Result{}, err: errTest})

	if got := len(a.result.Runners); got != 1 {
		t.Errorf("一覧の runner = %d 台, want 1（部分結果で上書きしている）", got)
	}
	if got := len(a.result.OrphanUnits); got != 1 {
		t.Errorf("孤児ユニット = %d 件, want 1（部分結果で上書きしている）", got)
	}
	if !strings.Contains(a.status(), errTest.Error()) {
		t.Errorf("状態行 = %q, 検出の警告が出ていない", a.status())
	}

	// page へ配られるスナップショットも直前の成功結果を保つ。
	st := spies[0].states[len(spies[0].states)-1]
	if len(st.Result.Runners) != 1 {
		t.Errorf("page へ配られた runner = %d 台, want 1", len(st.Result.Runners))
	}
	if st.Err == nil {
		t.Error("page へ検出のエラーが配られていない")
	}

	// 成功した周期では置き換える。
	a, _ = update(a, discoveredMsg{result: runner.Result{}, err: nil})
	if len(a.result.Runners) != 0 || a.err != nil {
		t.Errorf("成功した周期で結果が更新されていない: %+v / %v", a.result, a.err)
	}
}

// 検出の deadline は自動更新間隔から切り離す。
//
// 間隔と同値だと --refresh 1 でほぼ毎周期が期限切れになり、部分結果しか得られない。
func TestDiscoverBudgetIsDecoupledFromRefresh(t *testing.T) {
	a := newApp(exec.NewFake())
	a.opts.Refresh = minRefresh
	if got := a.refresh(); discoverBudget <= got {
		t.Errorf("検出の予算 = %v, want 更新間隔（%v）より長い", discoverBudget, got)
	}
}

// WindowSizeMsg は本体領域に換算してから配られる。
func TestWindowSizeDistributesBodySize(t *testing.T) {
	a, spies := withSpies(newApp(exec.NewFake()))
	const w, h = 100, 30

	_, _ = update(a, tea.WindowSizeMsg{Width: w, Height: h})
	wantW, wantH := template.BodySize(w, h)
	for i, s := range spies {
		if len(s.states) != 1 {
			t.Fatalf("タブ %d が受け取った StateMsg = %d 件, want 1", i, len(s.states))
		}
		st := s.states[0]
		if st.BodyW != wantW || st.BodyH != wantH {
			t.Errorf("タブ %d に配られた領域 = %dx%d, want %dx%d", i, st.BodyW, st.BodyH, wantW, wantH)
		}
		if st.BodyH >= h {
			t.Errorf("タブ %d に端末の高さがそのまま配られている", i)
		}
	}
}

// 背景色の応答で配色を解決し直し、全タブへ配る。
func TestBackgroundColorResolvesStyles(t *testing.T) {
	a, spies := withSpies(newApp(exec.NewFake()))
	if !a.dark {
		t.Fatal("初期値が暗背景になっていない（応答が来ない端末での既定）")
	}

	a, _ = update(a, tea.BackgroundColorMsg{Color: color.White})
	if a.dark {
		t.Error("明背景の応答で dark が偽になっていない")
	}
	for i, s := range spies {
		if len(s.states) != 1 || s.states[0].Dark {
			t.Errorf("タブ %d に明背景の配色が配られていない", i)
		}
	}
}

// ChromeMsg は有効タブのものだけを採用する。
func TestChromeFromActiveTabOnly(t *testing.T) {
	a, _ := withSpies(newApp(exec.NewFake()))

	a, _ = update(a, chromeWith(1, true, "絞り込み"))
	if a.chrome.Modal || a.chrome.Input != "" {
		t.Errorf("無効タブの ChromeMsg を採用している: %+v", a.chrome)
	}

	a, _ = update(a, chromeWith(0, true, "絞り込み"))
	if !a.chrome.Modal || a.chrome.Input != "絞り込み" {
		t.Errorf("有効タブの ChromeMsg を採用していない: %+v", a.chrome)
	}
}

// View は代替スクリーンを宣言し、共通レイアウトを返す。
func TestViewDeclaresAltScreen(t *testing.T) {
	a, _ := update(newApp(exec.NewFake()), tea.WindowSizeMsg{Width: 80, Height: 24})
	v := a.View()
	if !v.AltScreen {
		t.Error("AltScreen を宣言していない")
	}
	if n := len(strings.Split(v.Content, "\n")); n != 24 {
		t.Errorf("描画した行数 = %d, want 24（本体以外 %d 行の固定）", n, template.ChromeHeight)
	}
	for _, want := range []string{"gsr-helper", "build01", "Runners", "Jobs", "?:ヘルプ"} {
		if !strings.Contains(v.Content, want) {
			t.Errorf("%q が描かれていない", want)
		}
	}
}
