package confirmmodal

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/ui/organism/dialog"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/pagetest"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/pagetest/cmdtest"
)

// クリーンアップ確認ダイアログの包み方の回帰テスト（Issue #189）。
//
// **判断は 1 つで、`esc` の扱いである。** Setup タブの確認ダイアログは `esc` を
// 自分で受けるが（入力の破棄を問う 2 段階があるため）、こちらは `HandlesBack` を
// nil にして **`esc` を「1 枚閉じる＝キャンセル」に固定してある**（New の doc）。
// 閉じるだけで削除が走らないことは「実行の起点が
// `dialog.DecidedMsg{Confirmed: true}` の 1 本しか無いこと」で担保されるので、
// **閉じたときに承認が漏れ出さないこと**をここで縛る。
//
// 文面の組み立て（`confirmInput`）は disk パッケージ側にあり、ここは通すだけである。

// testTab は登録に使うタブ番号。0 以外にするのは、包み忘れ（page.Do を通さない）を
// 「たまたま 0 と一致する」で見逃さないためである。
const testTab = 3

// newOverlay は確認ダイアログを登録した Overlay と共有状態を返す。
func newOverlay(t *testing.T) (page.Overlay, page.StateMsg) {
	t.Helper()

	st := pagetest.State(80, 24)
	o, _ := page.NewOverlay(testTab, st)
	o.Register(Kind, New(st))

	return o, st
}

// cleanInput はクリーンアップの確認の文面。disk 側が組むものと同じ形にする。
func cleanInput() dialog.ConfirmInput {
	return dialog.ConfirmInput{
		Title:   "クリーンアップの確認",
		Targets: []string{"/opt/runners/build01-1/_work"},
		Impact:  []string{"1.2 GiB を解放します"},
		Command: []string{"docker", "system", "prune", "-f"},
		Note:    []string{"削除したファイルは復元できません"},
	}
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

// decideVia はキーを 1 打鍵ぶん配り、モーダルが自分宛に包んだ Msg を配り直して、
// page が受ける決定を返す。決定が出なければ第 2 返り値が偽になる。
//
// **2 段で辿るのが要点である。** 決定は「打鍵 → dialog が決定の Cmd を返す →
// モーダルが自分宛に包む → 配り直されて初めてモーダルが決定として受ける」という
// 往復を通る。1 段目を省いて dialog.DecidedMsg を直に流すと、包み（タブ番号と
// 種類）が壊れていても緑になる（modal.wrap の doc）。
func decideVia(t *testing.T, o *page.Overlay, k string) (page.ResultMsg, bool) {
	t.Helper()

	_, cmd := o.Update(pagetest.Press(k))
	if cmd == nil {
		return page.ResultMsg{Kind: "", Msg: nil}, false
	}

	var back tea.Cmd
	for _, msg := range cmdtest.MustMsgs(cmd, cmdtest.CmdTimeout) {
		tab, ok := msg.(page.TabMsg)
		if !ok {
			continue
		}
		if tab.Tab != testTab {
			t.Errorf("%q の包みの宛先 = タブ %d, want %d", k, tab.Tab, testTab)
		}
		to, ok := tab.Msg.(page.ModalMsg)
		if !ok {
			continue
		}
		if to.Kind != Kind {
			t.Errorf("%q の包みの種類 = %q, want %q", k, to.Kind, Kind)
		}
		_, back = o.Update(to)
	}
	if back == nil {
		return page.ResultMsg{Kind: "", Msg: nil}, false
	}

	return resultOf(t, back), true
}

// resultOf は Cmd を辿って page が受ける決定を返す。
func resultOf(t *testing.T, cmd tea.Cmd) page.ResultMsg {
	t.Helper()

	for _, msg := range cmdtest.MustMsgs(cmd, cmdtest.CmdTimeout) {
		tab, ok := msg.(page.TabMsg)
		if !ok {
			continue
		}
		res, ok := tab.Msg.(page.ResultMsg)
		if !ok {
			continue
		}
		if tab.Tab != testTab {
			t.Errorf("決定の差し戻し先 = タブ %d, want %d", tab.Tab, testTab)
		}
		return res
	}
	t.Fatal("page.TabMsg に包まれた page.ResultMsg が発行されていない")

	return page.ResultMsg{Kind: "", Msg: nil}
}

// y は承認として page へ差し戻される。
func TestYesIsForwardedAsConfirmedResult(t *testing.T) {
	t.Parallel()

	o, _ := newOverlay(t)
	Open(&o, cleanInput())

	res, ok := decideVia(t, &o, "y")
	if !ok {
		t.Fatal("y で決定が差し戻されていない")
	}
	if res.Kind != Kind {
		t.Errorf("決定の種類 = %q, want %q", res.Kind, Kind)
	}
	decided, isDecided := res.Msg.(dialog.DecidedMsg)
	if !isDecided {
		t.Fatalf("決定の中身 = %T, want dialog.DecidedMsg", res.Msg)
	}
	if !decided.Confirmed {
		t.Error("y が否認として差し戻された")
	}
}

// esc は Overlay が 1 枚閉じ、承認は漏れ出さない。
//
// **削除の起点は `Confirmed: true` の 1 本だけである**（New の doc）。閉じる操作で
// 承認が漏れると、キャンセルしたつもりの `esc` でクリーンアップが走る。
func TestBackKeyClosesWithoutConfirming(t *testing.T) {
	t.Parallel()

	o, _ := newOverlay(t)
	Open(&o, cleanInput())

	res, ok := decideVia(t, &o, "esc")

	if o.Active() {
		t.Error("esc でモーダルが閉じていない（キャンセルの手段が無い）")
	}
	if ok && res.Msg.(dialog.DecidedMsg).Confirmed {
		t.Error("esc が承認として差し戻された（キャンセルで削除が走る）")
	}
}

// n は否認として差し戻され、承認にはならない。
func TestNoIsForwardedAsRejection(t *testing.T) {
	t.Parallel()

	o, _ := newOverlay(t)
	Open(&o, cleanInput())

	res, ok := decideVia(t, &o, "n")
	if !ok {
		t.Fatal("n で決定が差し戻されていない")
	}
	decided, isDecided := res.Msg.(dialog.DecidedMsg)
	if !isDecided {
		t.Fatalf("決定の中身 = %T, want dialog.DecidedMsg", res.Msg)
	}
	if decided.Confirmed {
		t.Error("n が承認として差し戻された")
	}
}

// 開く指示に載せた文面がそのまま出る（組み立ては disk 側が持つ）。
func TestOpenShowsGivenInput(t *testing.T) {
	t.Parallel()

	o, _ := newOverlay(t)
	in := cleanInput()
	Open(&o, in)

	if got := title(modelOf(t, o)); got != in.Title {
		t.Errorf("見出し = %q, want %q", got, in.Title)
	}
	view := o.View()
	for _, want := range []string{in.Targets[0], in.Impact[0], in.Command[0], in.Note[0]} {
		if !strings.Contains(view, want) {
			t.Errorf("表示に %q が無い", want)
		}
	}
	if got := hints(modelOf(t, o)); len(got) == 0 {
		t.Error("フッタのキーヒントが空である")
	}
}

// 共有状態が届いても文面は消えない。
//
// 3 秒ごとに配られる共有状態でダイアログを作り直すと、確認の途中で文面が消えて
// 「何に対する y なのか」が読めなくなる（modal.Update の page.StateMsg の case）。
func TestStateDoesNotClearInput(t *testing.T) {
	t.Parallel()

	o, st := newOverlay(t)
	in := cleanInput()
	Open(&o, in)

	o.SetState(st)

	if got := title(modelOf(t, o)); got != in.Title {
		t.Errorf("共有状態の到着後の見出し = %q, want %q", got, in.Title)
	}
	if !strings.Contains(o.View(), in.Targets[0]) {
		t.Error("共有状態の到着で対象が消えた（作り直している）")
	}
}

// 見出しとフッタは他のモーダルの Model を渡されても落ちない。
//
// Overlay は登録した関数を Model 付きで呼ぶだけなので、型が合わない場合は
// 空を返すほかに手が無い（panic すると画面ごと落ちる）。
func TestTitleAndHintsIgnoreForeignModel(t *testing.T) {
	t.Parallel()

	foreign := pagetest.NewEcho()
	if got := title(foreign); got != "" {
		t.Errorf("別のモーダルの見出し = %q, want 空", got)
	}
	if got := hints(foreign); got != nil {
		t.Errorf("別のモーダルのフッタ = %v, want nil", got)
	}
}
