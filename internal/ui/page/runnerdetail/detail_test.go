package runnerdetail

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/appconfig"
	"github.com/ousiassllc/gsr-helper/internal/runner"
	"github.com/ousiassllc/gsr-helper/internal/ui/organism"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/pagetest"
	"github.com/ousiassllc/gsr-helper/internal/ui/token"
)

// newDetail は詳細画面を開いた状態で返す。
func newDetail() Model {
	d := newModel(pagetest.Keys(), pagetest.Styles())
	d.SetSize(72, 24)
	d.Open(pagetest.SampleRunner(), pagetest.Caps())
	return d
}

// sendDetail はキーを順に送り、最後の Cmd を返す。
func sendDetail(d Model, keys ...string) (Model, tea.Cmd) {
	var cmd tea.Cmd
	for _, k := range keys {
		d, cmd = d.Update(pagetest.Press(k))
	}
	return d, cmd
}

// 開き直すたびにカーソルは安全側の先頭へ戻る（FR-46）。
// 一覧の enter → 詳細の enter で破壊的操作に到達しないための規則である。
func TestRunnerDetailResetsCursorOnOpen(t *testing.T) {
	d, _ := sendDetail(newDetail(), "j", "j", "j")
	if d.Cursor() == 0 {
		t.Fatal("カーソルが動いていない")
	}

	d.Open(pagetest.BusyRunner(), pagetest.Caps())
	if got := d.Cursor(); got != 0 {
		t.Errorf("開き直した後のカーソル = %d, want 0", got)
	}

	// サイズの変更ではカーソルを戻さない（リサイズで選択が飛ぶと操作できない）。
	d, _ = sendDetail(d, "j")
	d.SetSize(60, 20)
	if got := d.Cursor(); got != 1 {
		t.Errorf("リサイズ後のカーソル = %d, want 1", got)
	}
}

// 情報部には screens.md の詳細画面の 7 項目が出る。
func TestRunnerDetailInfoItems(t *testing.T) {
	got := newDetail().View()
	for _, want := range []string{
		"スコープ", "起動方式", "サービス", "ジョブ", "バージョン", "ディレクトリ", "work",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("項目 %q が出ていない", want)
		}
	}

	r := pagetest.SampleRunner()
	for _, want := range []string{
		r.Scope.String(), r.UnitName, "active", "enabled", r.Version, "disableUpdate=true", r.WorkDir,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("値 %q が出ていない", want)
		}
	}
	if !strings.Contains(got, heading) {
		t.Errorf("操作リストの見出しが出ていない")
	}
}

// ジョブ実行中は経過時間と Worker の PID を出す。
func TestRunnerDetailBusyJob(t *testing.T) {
	d := newModel(pagetest.Keys(), pagetest.Styles())
	d.SetSize(72, 24)
	d.Open(pagetest.BusyRunner(), pagetest.Caps())

	got := d.View()
	for _, want := range []string{"実行中", "284193"} {
		if !strings.Contains(got, want) {
			t.Errorf("%q が出ていない", want)
		}
	}
	// ジョブが無いときの表記は一覧の JOB 列（atom.JobText）と同じにする。
	if !strings.Contains(newDetail().View(), "idle") {
		t.Error("ジョブが無いときに idle が出ていない")
	}
}

// 見出しは対象の runner 名を含む（Jobs タブから開いても対象が分かるようにする）。
func TestRunnerDetailTitle(t *testing.T) {
	if got := newDetail().Title(); !strings.Contains(got, "build01-1") {
		t.Errorf("見出し = %q, runner 名を含まない", got)
	}
}

// 操作キーの直打ちは操作リストへ配送され、実行できる項目では ChosenMsg が出る。
//
// この版の操作はすべて未対応（Allow が false を返す）なので、実行できる項目を
// 内側の ChoiceList へ差し込んで配送経路そのものを検証する。実装済みの操作が
// 増えても、経路は変わらない。
func TestRunnerDetailRoutesKeysToChoiceList(t *testing.T) {
	d := newDetail()
	d.list.SetItems([]organism.Choice{
		{Key: "l", Desc: "ログを開く", Impact: "", Reason: "", Enabled: true, DividerBefore: false},
		{Key: "D", Desc: "削除", Impact: "", Reason: "ジョブ実行中です", Enabled: false, DividerBefore: true},
	})

	_, cmd := sendDetail(d, "l")
	if cmd == nil {
		t.Fatal("実行できる操作のキーで Cmd が発行されない")
	}
	msg, ok := cmd().(organism.ChosenMsg)
	if !ok {
		t.Fatalf("ChosenMsg 以外の Msg が返った（%T）", cmd())
	}
	if msg.Key != "l" {
		t.Errorf("選ばれたキー = %q, want %q", msg.Key, "l")
	}

	if _, cmd := sendDetail(d, "D"); cmd != nil {
		t.Error("実行できない操作のキーで Cmd が発行されている")
	}
}

// この版では操作リストの全項目が実行できず、理由が添えられる。
func TestRunnerDetailShowsReasons(t *testing.T) {
	got := newDetail().View()
	if !strings.Contains(got, page.ReasonUnsupported) {
		t.Errorf("実行できない理由が出ていない")
	}
}

// フッタは詳細画面のキー（実行・選択・戻る）を出す。
func TestRunnerDetailHints(t *testing.T) {
	hints := newDetail().Hints()
	if len(hints) != 3 {
		t.Fatalf("ヒントの件数 = %d, want 3", len(hints))
	}
	for _, h := range hints {
		if h.Key == "" || h.Desc == "" {
			t.Errorf("ヒントに欠けがある: %+v", h)
		}
		if !h.Enabled {
			t.Errorf("ヒント %q が無効になっている", h.Key)
		}
	}
	if hints[0].Key != "enter" || hints[2].Key != "esc" {
		t.Errorf("ヒントのキー = %q..%q, want enter..esc", hints[0].Key, hints[2].Key)
	}
}

// 未稼働の runner は起動方式を "-" にせず、値なしと区別できる文で出す。
func TestRunnerDetailManagedUnknown(t *testing.T) {
	r := pagetest.SampleRunner()
	r.Managed, r.Svc, r.Listener, r.UnitName = runner.ManagedUnknown, nil, nil, ""

	d := newModel(pagetest.Keys(), pagetest.Styles())
	d.SetSize(80, 24)
	d.Open(r, pagetest.Caps())

	got := d.View()
	if !strings.Contains(got, managedUnknownText) {
		t.Errorf("起動方式 = %q, 未稼働であることが読み取れない", got)
	}

	// systemd 管理と run.sh 直起動はこれまでどおり短い表記で出す。
	d.Open(pagetest.SampleRunner(), pagetest.Caps())
	if !strings.Contains(d.View(), "systemd（"+pagetest.SampleRunner().UnitName+"）") {
		t.Error("systemd 管理の起動方式にユニット名が添えられていない")
	}
	d.Open(pagetest.StandaloneRunner(), pagetest.Caps())
	if !strings.Contains(d.View(), "run.sh") {
		t.Error("run.sh 直起動の起動方式が出ていない")
	}
}

// systemd の状態が判定できない runner は、未稼働（ユニットなし）と書き分ける。
//
// 記号だけ（? / -）だと、ユニットが無いのか分からないのかを読み分けられず、
// 利用者が「登録されていない」と誤って判断して登録し直しに向かってしまう。
func TestRunnerDetailManagedUnavailable(t *testing.T) {
	r := pagetest.SampleRunner()
	r.Managed, r.Svc, r.Listener, r.UnitName = runner.ManagedUnavailable, nil, nil, ""

	d := newModel(pagetest.Keys(), pagetest.Styles())
	d.SetSize(80, 24)
	d.Open(r, pagetest.Caps())

	got := d.View()
	if !strings.Contains(got, managedUnavailableText) {
		t.Errorf("起動方式 = %q, want %q（判定できないことの説明）", got, managedUnavailableText)
	}
	if strings.Contains(got, managedUnknownText) {
		t.Error("判定不能を未稼働と同じ文で出している")
	}
}

// 状態を取得できなかったユニットは、ユニットが無い場合と書き分ける。
func TestRunnerDetailUnknownServiceState(t *testing.T) {
	r := pagetest.SampleRunner()
	r.Svc = &runner.SvcState{Unit: r.UnitName}

	d := newModel(pagetest.Keys(), pagetest.Styles())
	d.SetSize(80, 24)
	d.Open(r, pagetest.Caps())

	if got := d.View(); !strings.Contains(got, token.IconUnknown) {
		t.Errorf("サービスの行 = %q, want %q を含む", got, token.IconUnknown)
	}
}

// SetState は届いた Caps をそのまま採る。
//
// 以前はゼロ値のときだけ握り続けるガードを置いていたが、これは「まだ検出していない」と
// 「この host は本当に全能力 false」を区別できない。加えて appconfig.Detect は起動時
// 1 回であり、page へ配られる StateMsg は常に検出済みの Caps を載せる
// （ui.App.state / newTabs）。ガードには発火する余地が無く、能力を持たないホストで
// 開いた時点の Caps を握り続ける危険だけが残っていた。
func TestRunnerDetailAdoptsCapsFromState(t *testing.T) {
	d := newDetail()
	before := d.View()

	// 全能力 false のホスト（root でも systemd でもない）。
	st := pagetest.State(72, 24, pagetest.SampleRunner())
	st.Caps = appconfig.Caps{}
	d.SetState(st)

	if d.caps != (appconfig.Caps{}) {
		t.Errorf("Caps = %+v, want ゼロ値（届いた Caps をそのまま採る）", d.caps)
	}
	if d.View() == before {
		t.Error("全能力 false の Caps が操作リストに反映されていない")
	}

	// 能力が戻れば操作リストも戻る。
	st.Caps = pagetest.Caps()
	d.SetState(st)
	if d.caps != pagetest.Caps() {
		t.Errorf("Caps = %+v, want %+v", d.caps, pagetest.Caps())
	}
}
