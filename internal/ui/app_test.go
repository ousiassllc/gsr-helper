package ui

import (
	"errors"
	"image/color"
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/appconfig"
	"github.com/ousiassllc/gsr-helper/internal/audit"
	"github.com/ousiassllc/gsr-helper/internal/exec"
	"github.com/ousiassllc/gsr-helper/internal/runner"
	"github.com/ousiassllc/gsr-helper/internal/ui/discovery"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/pagetest"
	"github.com/ousiassllc/gsr-helper/internal/ui/template"
)

// Init は背景色の問い合わせと最初の周期の Tick の 2 本を発行する。
//
// 検出は Init から直に発行しない（実行中の本数を親が数えられないため。Init の doc）。
func TestInitEmitsBackgroundColorAndFirstTick(t *testing.T) {
	fake := exec.NewFake()
	cmds := pagetest.Expand(newApp(fake).Init())
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

// 最初の tickMsg で検出と次の Tick が発行され、検出は Executor を使う（systemd が
// ある能力なので systemctl を叩く）。
//
// **Cmd を実行する前に Executor の呼び出しを数える。** 重い処理はすべて tea.Cmd と
// して UI の外で走らせる約束であり、Update の中でドメイン層を呼んでいないことは
// 実行前の 0 件でしか見分けられない。
func TestFirstTickRunsDiscover(t *testing.T) {
	fake := exec.NewFake()
	a, cmd := update(newApp(fake), tickMsg{})
	cmds := pagetest.Expand(cmd)
	if len(cmds) != 2 {
		t.Fatalf("tickMsg が発行した Cmd の本数 = %d, want 2（検出 + 次の Tick）", len(cmds))
	}
	if n := len(fake.Calls()); n != 0 {
		t.Errorf("Update の中でドメイン層を呼んでいる（Executor の呼び出し %d 件）", n)
	}
	if _, ok := cmds[0]().(discovery.Msg); !ok {
		t.Errorf("1 本目の Msg = %T, want discovery.Msg", cmds[0]())
	}
	if len(fake.Calls()) == 0 {
		t.Error("検出が Executor を使っていない")
	}
	if a.inflight != 1 {
		t.Errorf("実行中の検出の本数 = %d, want 1", a.inflight)
	}
}

// systemctl が無い環境では Executor を渡さず、systemd を参照しない。
//
// nil を返す判定そのものは discovery.Exec が持つ（discovery/discovery_test.go の
// TestExecReturnsNilWithoutSystemd）。ここでは a.discover() がその判定を実際に
// 使っていることだけを見る。
func TestDiscoverWithoutSystemd(t *testing.T) {
	fake := exec.NewFake()
	a := newApp(fake)
	a.caps.Systemd = false

	a.discover()()
	if n := len(fake.Calls()); n != 0 {
		t.Errorf("systemd が無いのにコマンドを %d 件発行している", n)
	}
}

// discovery.Msg は有効な全タブへ配られる。
func TestDiscoveredDistributesToAllTabs(t *testing.T) {
	a, spies := withSpies(newApp(exec.NewFake()))
	res := runner.Result{
		Runners:     nil,
		OrphanUnits: []runner.SvcState{{Unit: "actions.runner.foo.old.service"}},
		Warnings:    nil,
	}

	a, _ = update(a, discovery.Msg{Result: res, Err: nil})
	for i, s := range spies {
		if len(s.States()) != 1 {
			t.Fatalf("タブ %d が受け取った StateMsg = %d 件, want 1", i, len(s.States()))
		}
		if got := len(s.States()[0].Result.OrphanUnits); got != 1 {
			t.Errorf("タブ %d に配られた孤児ユニット = %d 件, want 1", i, got)
		}
	}

	// 孤児ユニットの件数は親が状態行に出す。
	if got := statusLine(a); !strings.Contains(got, "孤児ユニット 1 件") {
		t.Errorf("状態行 = %q, 孤児ユニットの件数が無い", got)
	}
}

// errTest は検出のエラーを模した値。
var errTest = errors.New("検出が間に合いませんでした")

// 期限切れ・失敗した周期の部分結果で直前の成功結果を上書きせず、エラーは状態行に
// 出し、画面遷移は巻き戻さない。
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
	a, _ = update(a, discovery.Msg{Result: success, Err: nil})

	// 期限切れの周期。取れた分だけの部分結果（runner 0 台・孤児 0 件）が届く。
	a, _ = update(a, discovery.Msg{Result: runner.Result{}, Err: errTest})

	if got := len(a.result.Runners); got != 1 {
		t.Errorf("一覧の runner = %d 台, want 1（部分結果で上書きしている）", got)
	}
	if got := len(a.result.OrphanUnits); got != 1 {
		t.Errorf("孤児ユニット = %d 件, want 1（部分結果で上書きしている）", got)
	}
	if !strings.Contains(statusLine(a), errTest.Error()) {
		t.Errorf("状態行 = %q, 検出の警告が出ていない", statusLine(a))
	}
	if a.active != 0 {
		t.Errorf("エラーでタブが変わっている（active = %d）", a.active)
	}

	// page へ配られるスナップショットも直前の成功結果を保つ。
	st := spies[0].States()[len(spies[0].States())-1]
	if len(st.Result.Runners) != 1 {
		t.Errorf("page へ配られた runner = %d 台, want 1", len(st.Result.Runners))
	}
	if st.Err == nil {
		t.Error("page へ検出のエラーが配られていない")
	}

	// 成功した周期では置き換える。
	a, _ = update(a, discovery.Msg{Result: runner.Result{}, Err: nil})
	if len(a.result.Runners) != 0 || a.err != nil {
		t.Errorf("成功した周期で結果が更新されていない: %+v / %v", a.result, a.err)
	}
}

// 検出の deadline を自動更新間隔から切り離す判定そのものは discovery.Interval /
// discovery.Budget の責務になった（discovery/interval_test.go の
// TestBudgetIsDecoupledFromInterval を参照）。ここでは a.refresh() が discovery.Interval
// への薄い委譲のままであることだけを確かめる。
func TestRefreshDelegatesToDiscoveryInterval(t *testing.T) {
	a := newApp(exec.NewFake())
	a.opts.Refresh = discovery.MinRefresh
	want := discovery.Interval(a.opts.Refresh, a.cfg.RefreshDuration())
	if got := a.refresh(); got != want {
		t.Errorf("a.refresh() = %v, want discovery.Interval と同じ %v", got, want)
	}
}

// WindowSizeMsg は本体領域に換算してから配られる。
func TestWindowSizeDistributesBodySize(t *testing.T) {
	a, spies := withSpies(newApp(exec.NewFake()))
	const w, h = 100, 30

	_, _ = update(a, tea.WindowSizeMsg{Width: w, Height: h})
	wantW, wantH := template.BodySize(w, h)
	for i, s := range spies {
		if len(s.States()) != 1 {
			t.Fatalf("タブ %d が受け取った StateMsg = %d 件, want 1", i, len(s.States()))
		}
		st := s.States()[0]
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
		if len(s.States()) != 1 || s.States()[0].Dark {
			t.Errorf("タブ %d に明背景の配色が配られていない", i)
		}
	}
}

// ChromeMsg は有効タブのものだけを採用する。
func TestChromeFromActiveTabOnly(t *testing.T) {
	a, _ := withSpies(newApp(exec.NewFake()))
	// 空の ChromeMsg（pageChrome）にモーダルと入力中を立てたもの。
	filtering := func(tab int) page.ChromeMsg {
		c := pageChrome(tab)
		c.Modal, c.Input = true, "絞り込み"
		return c
	}

	a, _ = update(a, filtering(1))
	if a.chrome.Modal || a.chrome.Input != "" {
		t.Errorf("無効タブの ChromeMsg を採用している: %+v", a.chrome)
	}

	a, _ = update(a, filtering(0))
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

// 書き込めた設定は以後の共有状態へ載る（Issue #128）。載せ直さないと、Disk タブの
// 警告閾値超過と doctor のリソース診断が再起動まで起動時の閾値で判定し続ける。
func TestConfigSavedUpdatesSharedState(t *testing.T) {
	cfg := appconfig.Default()
	cfg.DiskThresholds = appconfig.DiskThresholds{Warn: 55, Critical: 77}
	a, cmd := update(newApp(exec.NewFake()), page.ConfigSavedMsg{Conf: cfg})

	if got := a.state().Disk.Thresholds; got != cfg.DiskThresholds {
		t.Errorf("保存後の StateMsg.Disk.Thresholds = %+v, want %+v", got, cfg.DiskThresholds)
	}
	if cmd == nil {
		t.Error("保存後に共有状態を配り直していない（開いている page が古い閾値のまま）")
	}
}

// 起動時に決まる値は共有状態へ載り、まだ確定していない値は載らない。
//
// 組み立ては 1 つ（New → state）なので 1 本にまとめてある。表明ごとの理由は次のとおり。
func TestStateCarriesStartupValues(t *testing.T) {
	lg := audit.Discard()
	cfg := appconfig.Default()
	cfg.DiskThresholds = appconfig.DiskThresholds{Warn: 55, Critical: 77}
	st := New(cfg, appconfig.Caps{}, exec.NewFake(), Options{Audit: lg}).state()

	// 監査ログの記録先（Issue #71）。載らないと、外部コマンドを伴わない削除
	// （internal/disk のファイル削除）が記録先を持てず、確認を経た破壊的操作が
	// 監査ログに 1 行も残らない。
	if st.Audit != lg {
		t.Errorf("StateMsg.Audit = %v, want 渡した Logger（記録先が page へ届いていない）", st.Audit)
	}
	// 設定のディスク閾値（Issue #72）。載らないと Disk タブの要約行が閾値を判定
	// できず、既定値を表示側に埋め込むことになる。
	if st.Disk.Thresholds != cfg.DiskThresholds {
		t.Errorf("StateMsg.Disk.Thresholds = %+v, want %+v", st.Disk.Thresholds, cfg.DiskThresholds)
	}
	// _work 使用量と保有スコープ（Issue #73 / #79）は確定するまで載せない。未集計を
	// 0 バイトとして、判定前を「スコープ無し」として配ると、一覧が誤った使用量を
	// 出し、権限のあるトークンの操作が塞がれる。
	if len(st.Disk.Work) != 0 {
		t.Errorf("StateMsg.Disk.Work = %v, want 空（未集計はキーを持たない）", st.Disk.Work)
	}
	if st.Scopes.Known {
		t.Error("StateMsg.Scopes.Known = true, want false（まだ引いていない）")
	}
}
