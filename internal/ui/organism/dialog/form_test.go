package dialog_test

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"

	"github.com/ousiassllc/gsr-helper/internal/ui/organism/dialog"
)

// formSteps は完了まで huh の Msg を辿る段数の上限。
//
// 上限を置くのは、辿る先に終わらない Msg（カーソルの点滅）が混じっても
// テストが止まらないようにするためである。
const formSteps = 8

// newTestForm は 1 項目のフォームを載せたラッパーを返す。
func newTestForm(t *testing.T, count *string) dialog.Form {
	t.Helper()

	f := dialog.NewForm(testStyles(), false)
	f.SetTitle("runner の追加")
	f.SetSize(60, 12)
	f.SetForm(huh.NewForm(huh.NewGroup(
		huh.NewInput().Key("count").Title("台数").Value(count),
	)))
	return f
}

// typeKeys は打鍵を順に配る。返る Cmd は実行しない。
//
// 文字を打つと bubbles/textinput がカーソルの点滅の Cmd を返す。実行すると点滅の
// 間隔（0.5 秒）だけ待たされ、返った Msg を配り直すとまた点滅の Cmd が返るため
// テストが回り続ける。入力の検証に点滅は要らない。
func typeKeys(f dialog.Form, keys ...string) dialog.Form {
	for _, k := range keys {
		f, _ = f.Update(press(k))
	}
	return f
}

// notify は Msg を 1 つ配り、返った Cmd の Msg を取り出す。Cmd が無ければ nil を返す。
func notify(f dialog.Form, msg tea.Msg) (dialog.Form, tea.Msg) {
	f, cmd := f.Update(msg)
	if cmd == nil {
		return f, nil
	}
	return f, cmd()
}

// submit は入力を終える Msg を配り、huh が自分へ返す Msg を辿って通知を取り出す。
//
// huh は項目の送り・区画の送り・送信を Msg で自分へ返す作りなので、1 段だけ辿っても
// 完了の通知には届かない。
func submit(t *testing.T, f dialog.Form) (dialog.Form, tea.Msg) {
	t.Helper()

	msg := huh.NextField()
	for range formSteps {
		var next tea.Msg
		f, next = notify(f, msg)
		switch m := next.(type) {
		case dialog.FormDoneMsg, dialog.FormAbortedMsg, dialog.FormDiscardMsg:
			return f, m
		case nil:
			return f, nil
		default:
			msg = m
		}
	}
	return f, nil
}

// 入力が空のまま esc を押すと、確認を挟まず中断が確定する。
//
// 1 文字も打っていない画面で破棄の確認を出すと、戻るのに 2 打鍵かかる
// （atomic-design.md の「`Form` と huh」）。
func TestFormEscWithoutInputAborts(t *testing.T) {
	var count string

	f := newTestForm(t, &count)
	if f.Dirty() {
		t.Error("何も打っていないのに入力済みになっている")
	}

	f, msg := notify(f, press("esc"))
	if _, ok := msg.(dialog.FormAbortedMsg); !ok {
		t.Errorf("esc の通知 = %T, want dialog.FormAbortedMsg", msg)
	}
	if f.Dirty() {
		t.Error("esc で入力済みの印が立った")
	}
}

// 入力済みの項目があるときの esc は、破棄の確認を求める。
//
// 打ち間違いの esc で入力全体を失わせないためである。**確認そのものはここで行わない。**
// 確認ダイアログの実装は Confirm 1 つに統一する決まりであり、page が Confirm を重ねる。
func TestFormEscAfterInputAsksDiscard(t *testing.T) {
	var count string

	f := typeKeys(newTestForm(t, &count), "3")
	if !f.Dirty() {
		t.Fatal("入力しても入力済みにならない")
	}

	_, msg := notify(f, press("esc"))
	if _, ok := msg.(dialog.FormDiscardMsg); !ok {
		t.Errorf("入力済みの esc の通知 = %T, want dialog.FormDiscardMsg", msg)
	}
}

// 項目の移動だけでは入力済みにならない。
//
// 既定値の違う項目へ移っただけで「入力済み」になると、素通りしただけの画面で
// esc が破棄の確認を出す。
func TestFormNavigationIsNotInput(t *testing.T) {
	var first, second string

	f := dialog.NewForm(testStyles(), false)
	f.SetSize(60, 12)
	f.SetForm(huh.NewForm(huh.NewGroup(
		huh.NewInput().Key("first").Value(&first),
		huh.NewInput().Key("second").Value(&second),
	)))

	for _, msg := range []tea.Msg{huh.NextField(), huh.PrevField()} {
		f, _ = f.Update(msg)
	}
	if f.Dirty() {
		t.Error("移動しただけで入力済みになっている")
	}

	// 移った先で打てば入力済みになる。
	f, _ = f.Update(huh.NextField())
	if f = typeKeys(f, "x"); !f.Dirty() {
		t.Error("2 つ目の項目への入力を取りこぼしている")
	}
}

// 入力を終えると、値を載せた完了の通知が届く。
//
// page はこれを受けてドメイン層の Cmd を発行する。**Form 自身はドメインを呼ばない**
// （atomic-design.md の「`Form` と huh」）。
func TestFormNotifiesDoneWithValues(t *testing.T) {
	var count string

	f := typeKeys(newTestForm(t, &count), "3")

	f, msg := submit(t, f)
	done, ok := msg.(dialog.FormDoneMsg)
	if !ok {
		t.Fatalf("完了の通知 = %T, want dialog.FormDoneMsg", msg)
	}
	if got := done.Form.GetString("count"); got != "3" {
		t.Errorf("通知に載った値 = %q, want %q", got, "3")
	}
	if count != "3" {
		t.Errorf("束縛した変数 = %q, want %q", count, "3")
	}
	// 見出しは枠（template.Modal）が描くため、完了後も残る。
	if f.Title() != "runner の追加" {
		t.Errorf("Title = %q, want %q", f.Title(), "runner の追加")
	}
}

// フォームを差し替えると入力済みの印が落ちる。
//
// 前回の入力状態が残っていると、1 文字も打っていない画面で esc が破棄の確認を出す。
func TestFormSetFormResetsDirty(t *testing.T) {
	var count string

	f := typeKeys(newTestForm(t, &count), "3")
	if !f.Dirty() {
		t.Fatal("入力しても入力済みにならない")
	}

	var next string
	f.SetForm(huh.NewForm(huh.NewGroup(huh.NewInput().Key("next").Value(&next))))
	if f.Dirty() {
		t.Error("フォームを差し替えても入力済みの印が残っている")
	}
}

// フッタに出すのは esc だけで、入力済みかどうかで説明が変わる。
//
// 項目の移動・確定・選択のキーは項目の種類で変わり、それを描けるのは huh だけである。
// 書き写すと同じキーがフッタとフォームの 2 か所に別々の表記で並ぶ。
func TestFormHints(t *testing.T) {
	var count string

	f := newTestForm(t, &count)
	hints := f.Hints()
	if len(hints) != 1 || hints[0].Key != "esc" {
		t.Fatalf("Hints = %+v, want esc の 1 つだけ", hints)
	}
	if hints[0].Desc != "戻る" {
		t.Errorf("入力前の説明 = %q, want %q", hints[0].Desc, "戻る")
	}

	f = typeKeys(f, "3")
	if got := f.Hints()[0].Desc; got != "破棄して戻る" {
		t.Errorf("入力後の説明 = %q, want %q", got, "破棄して戻る")
	}
}

// フォームが未設定でも panic せず、何も描かない。
func TestFormWithoutFormIsInert(t *testing.T) {
	f := dialog.NewForm(testStyles(), false)
	f.SetSize(60, 12)

	if got := f.View(); got != "" {
		t.Errorf("View = %q, want 空", got)
	}
	if _, msg := notify(f, press("esc")); msg != nil {
		t.Errorf("未設定のフォームが通知を出した: %T", msg)
	}
}

// 色を使わない指定のときは描画に色が出ない。
//
// NO_COLOR の縮退は token.HuhTheme と同じ経路で伝わる（atomic-design.md の
// 「`Form` と huh」）。反転（`ESC[7m`。入力欄のカーソル）は色ではないので残る。
func TestFormNoColorView(t *testing.T) {
	var count string

	view := newTestForm(t, &count).View()
	for _, sgr := range []string{"\x1b[38;", "\x1b[48;", "\x1b[3", "\x1b[4"} {
		if strings.Contains(view, sgr) {
			t.Errorf("色を使わない指定なのに %q が出ている: %q", sgr, view)
		}
	}
	// 幅を渡してあるので、モーダルの枠からはみ出す行は出ない。
	wantNoWideLine(t, view, 60)
}
