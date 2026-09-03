package progressmodal

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/ui/organism/pane"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/pagetest"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/pagetest/cmdtest"
)

// 進捗表示の包み方の回帰テスト（Issue #188）。
//
// **判断は 2 つある。** 1 つは `esc` を握りつぶすかを実行中かどうかで決めること
// （閉じても処理は止まらないので、実行中に閉じると進捗を見失うだけになる。New の
// doc）。もう 1 つはスピナを回す Cmd を**開いたときにだけ**流すことで、差し替えの
// たびに流すと進捗 1 件ごとに Tick が積み増して描画が加速する。
//
// 中身（pane.ProgressList）の検証はあちらが持つ。ここが見るのは包み方だけである。

// testTab は登録に使うタブ番号。0 以外にするのは、包み忘れ（page.WrapModal を
// 通さない）を「たまたま 0 と一致する」で見逃さないためである。
const testTab = 3

// newOverlay は進捗表示を登録した Overlay と共有状態を返す。
func newOverlay(t *testing.T) (page.Overlay, page.StateMsg) {
	t.Helper()

	st := pagetest.State(80, 24)
	o, _ := page.NewOverlay(testTab, st)
	o.Register(Kind, New(st))

	return o, st
}

// input は件数の分かっている進捗を組む。
func input(title string, done int, report ...string) pane.ProgressInput {
	return pane.ProgressInput{Title: title, Rows: nil, Done: done, Total: 3, Report: report}
}

// modelOf は登録済みモーダルの中身を返す。
func modelOf(t *testing.T, o page.Overlay) tea.Model {
	t.Helper()

	m, ok := o.Modal(Kind)
	if !ok {
		t.Fatalf("%q が登録されていない", Kind)
	}
	return m.Model
}

// wrappedMsgs は Cmd を辿り、この進捗表示宛に包まれた Msg を返す。
//
// **包みごと確かめる。** モーダルが自分で発行した Cmd は page.WrapModal で
// タブ番号と種類を載せて包まなければ、結果が「そのとき選択中のタブの最上位の
// モーダル」へ配られてスピナが自分へ戻らない（page.WrapModal の doc）。
func wrappedMsgs(t *testing.T, cmd tea.Cmd) []tea.Msg {
	t.Helper()

	if cmd == nil {
		return nil
	}

	out := make([]tea.Msg, 0, 4)
	for _, msg := range cmdtest.MustMsgs(cmd, cmdtest.CmdTimeout) {
		tab, ok := msg.(page.TabMsg)
		if !ok {
			continue
		}
		if tab.Tab != testTab {
			t.Errorf("包みの宛先 = タブ %d, want %d", tab.Tab, testTab)
		}
		to, ok := tab.Msg.(page.ModalMsg)
		if !ok {
			continue
		}
		if to.Kind != Kind {
			t.Errorf("包みの種類 = %q, want %q", to.Kind, Kind)
		}
		out = append(out, to.Msg)
	}
	return out
}

// スピナを回す Cmd は開いたときにだけ流れ、差し替えでは流れない。
//
// 差し替えでも流すと、逐次の完了通知 1 件ごとに Tick が 1 本増える。Tick は自分の
// 次の Tick を繋いで回り続けるので（pane.ProgressList.Stop の doc）、増えたぶんは
// 止めるまで減らない。
//
// **差し替えと停止は Cmd 自体を nil で縛る。** 包まれた Msg を数える形（wrappedMsgs）は
// ここでは使えない——包み忘れた Cmd は page.TabMsg に入らないので数に上がらず、
// 「Cmd が nil」と「page.WrapModal を通さない Cmd が流れた」を区別できないからである
// （SetMsg の case を `return m, m.list.Start()` へ改変すると、Tick は実際に積み増すのに
// 数は 0 本のままになる）。どちらも約束は「Cmd を 1 本も返さない」ことなので
// （SetMsg の case は nil を返し、pane.ProgressList.Stop の戻り値も常に nil である）、
// 包みの中身ではなく Cmd の有無で見る。
func TestSpinnerStartsOnlyWhenOpened(t *testing.T) {
	t.Parallel()

	o, _ := newOverlay(t)

	if got := wrappedMsgs(t, Open(&o, input("追加中…", 0))); len(got) == 0 {
		t.Error("開いてもスピナを回す Cmd が流れていない")
	}
	if got := Set(&o, input("追加中…", 1)); got != nil {
		t.Error("差し替えで Cmd が流れた, want nil（Tick が積み増す）")
	}
	if got := Stop(&o); got != nil {
		t.Error("停止で Cmd が流れた, want nil")
	}
}

// esc は実行中だけ握りつぶし、終わったら 1 枚閉じる。
//
// 閉じても処理は止まらないため、実行中に閉じられると進捗を見失うだけになる。
// 逆に終わった後も握りつぶすと、閉じる手段が無くなって画面から出られない。
func TestBackKeyIsBlockedOnlyWhileRunning(t *testing.T) {
	t.Parallel()

	o, _ := newOverlay(t)
	Open(&o, input("追加中…", 0))

	o.Update(pagetest.Press("esc"))
	if !o.Active() {
		t.Fatal("実行中の esc でモーダルが閉じた（進捗を見失う）")
	}

	Stop(&o)

	o.Update(pagetest.Press("esc"))
	if o.Active() {
		t.Error("実行が終わった後の esc で閉じない（画面から出られない）")
	}
}

// 差し替えはモーダルを積み増さない。
//
// 開き直す実装だと、進捗 1 件ごとに 1 枚積まれて数百枚が溜まる（Set の doc）。
// esc 1 回で閉じきることで、積まれていないことを見る。
func TestSetDoesNotStackModals(t *testing.T) {
	t.Parallel()

	o, _ := newOverlay(t)
	Open(&o, input("追加中…", 0))
	for i := 1; i <= 3; i++ {
		Set(&o, input("追加中…", i))
	}
	Stop(&o)

	o.Update(pagetest.Press("esc"))
	if o.Active() {
		t.Error("esc 1 回で閉じきれない（差し替えのたびに積み増している）")
	}
}

// 差し替えた中身が表示に出て、実行中の扱いは変わらない。
func TestSetReplacesContentAndKeepsRunning(t *testing.T) {
	t.Parallel()

	o, _ := newOverlay(t)
	Open(&o, input("追加中…", 0, "build01-1: 準備中"))
	Set(&o, input("追加中…", 2, "build01-2: 完了"))

	view := o.View()
	if !strings.Contains(view, "build01-2: 完了") {
		t.Error("差し替えた結果報告が表示されていない")
	}
	if strings.Contains(view, "build01-1: 準備中") {
		t.Error("差し替える前の結果報告が残っている")
	}
	if !handlesBack(modelOf(t, o)) {
		t.Error("差し替えで実行中の扱いが解けた（esc で閉じられてしまう）")
	}
}

// フッタは実行中かどうかで入れ替わる。
//
// 実行中に「閉じる」と出すと、押しても閉じないキーを案内することになる
// （esc は上のとおり握りつぶされる）。
func TestHintsFollowRunning(t *testing.T) {
	t.Parallel()

	o, st := newOverlay(t)
	Open(&o, input("追加中…", 0))

	running := hints(modelOf(t, o))
	if len(running) != 1 {
		t.Fatalf("実行中のフッタ = %d 件, want 1", len(running))
	}
	if running[0].Enabled || running[0].Key != "" {
		t.Errorf("実行中のフッタ = %+v, want 押せないキー無しの案内", running[0])
	}

	Stop(&o)

	stopped := hints(modelOf(t, o))
	if len(stopped) != 1 {
		t.Fatalf("停止後のフッタ = %d 件, want 1", len(stopped))
	}
	if !stopped[0].Enabled {
		t.Errorf("停止後のフッタ = %+v, want 押せる案内", stopped[0])
	}
	if want := page.BindingKey(st.Keys.Global.Back); stopped[0].Key != want {
		t.Errorf("停止後のフッタのキー = %q, want %q", stopped[0].Key, want)
	}
}

// 見出しは常に同じで、フッタと esc の判定は他のモーダルの Model でも落ちない。
//
// Overlay は登録した関数を Model 付きで呼ぶだけなので、型が合わない場合は
// 空を返すほかに手が無い（panic すると画面ごと落ちる）。
func TestTitleAndGuardsAgainstForeignModel(t *testing.T) {
	t.Parallel()

	foreign := pagetest.NewEcho()
	if got := title(foreign); got == "" {
		t.Error("見出しが空である")
	}
	if got := hints(foreign); got != nil {
		t.Errorf("別のモーダルのフッタ = %v, want nil", got)
	}
	if handlesBack(foreign) {
		t.Error("別のモーダルで esc を握りつぶしている")
	}
}
