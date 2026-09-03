package setupmodal

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"

	"github.com/ousiassllc/gsr-helper/internal/ui/organism/dialog"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/pagetest"
	"github.com/ousiassllc/gsr-helper/internal/ui/token"
)

// 追加フォームの包み方の回帰テスト（Issue #187）。
//
// **判断は 3 つある。** 完了・中断・破棄の 3 つをどれも同じ種類（FormKind）で
// 差し戻すこと（page 側が 1 つの case で受けられるのはそのためである）、`esc` を
// Overlay ではなく dialog.Form に受けさせること（入力済みなら破棄の確認を出すため。
// NewForm が HandlesBack に真を返させている理由）、そして**組み立て関数へ渡す**
// テーマを開くときに渡された共有状態から組むことである。3 つめを登録時の値で組むと、
// **組み立て関数が受け取るテーマだけが古くなる**——入力欄の描画そのものは古くならない。
// 内側が最終的に着る配色は別の経路（page.Overlay.Open が formOpenMsg の直前に流す
// page.StateMsg のリプレイ）で最新に揃うためである。機構は
// TestOpenFormPassesThemeBuiltFromGivenState の doc に書いた。

// formTitleText は開く指示に載せる見出し。
const formTitleText = "runner の追加"

// probeForm は 1 欄だけのフォームを、渡されたテーマで組む。
//
// テーマを外へ持ち出すのは、開く指示に載せた共有状態が届いたかを見るためである
// （組み立て関数を受け取る形にしてある理由が formOpenMsg の doc にある）。
func probeForm(got *huh.Theme) func(huh.Theme) *huh.Form {
	return func(th huh.Theme) *huh.Form {
		*got = th

		return huh.NewForm(huh.NewGroup(
			huh.NewInput().Title("名前").Value(new(string)),
		)).WithTheme(th)
	}
}

// newFormOverlay は色を使わない共有状態で追加フォームを登録した Overlay を返す。
func newFormOverlay(t *testing.T) (page.Overlay, page.StateMsg) {
	t.Helper()

	st := state()
	if st.Color {
		t.Fatal("既定の共有状態が色を使う（登録時と開く時の差が作れない）")
	}
	o, _ := page.NewOverlay(testTab, st)
	o.Register(FormKind, NewForm(st))

	return o, st
}

// 完了は FormKind の決定として差し戻される。
//
// 別の種類（あるいは包み忘れ）になると、page はその結末を受け取れず、
// **フォームが閉じないまま残る**。
//
// **完了だけは Msg を直に流す。** 完了は huh 自身の差込口（huh.Form.SubmitCmd）から
// 生まれるので、打鍵から辿るには huh の全項目を埋めて確定させる必要があり、
// このモーダルの包み方とは別の層（huh の項目送り）を検査に巻き込むことになる。
// esc から生まれる残りの 2 つ（中断・破棄）は打鍵から辿る形で下の
// TestFormEscIsHandledByFormNotOverlay が縛る。
func TestFormDoneCarriesFormKind(t *testing.T) {
	t.Parallel()

	o, st := newFormOverlay(t)
	var th huh.Theme
	OpenForm(&o, st, formTitleText, probeForm(&th))

	done := dialog.FormDoneMsg{Form: nil}
	_, cmd := o.Update(done)

	res := resultOf(t, cmd)
	if res.Kind != FormKind {
		t.Errorf("決定の種類 = %q, want %q", res.Kind, FormKind)
	}
	if got, want := res.Msg, tea.Msg(done); got != want {
		t.Errorf("決定の中身 = %T(%v), want %T(%v)", got, got, want, want)
	}
}

// esc は Overlay ではなく dialog.Form が先に受ける。
//
// **Overlay に閉じさせてはならない。** 入力済みなら破棄の確認を出すという判断は
// dialog.Form が持つので（form.go の NewForm が HandlesBack に真を返させている理由）、
// Overlay が食って 1 枚閉じると**入力が確認なしに消える**。確認ダイアログ側には
// 対応する TestConfirmHandlesBackItself があり、フォーム側はこれである。
//
// esc から生まれる Msg は入力の有無で 2 つに分かれるので、どちらも見る——未入力なら
// 中断（そのまま前の画面へ戻ってよい）、入力済みなら破棄の確認である。
//
// **打鍵から辿る（decideVia）。** 生まれる Msg を o.Update へ直に流すと、esc が
// Overlay に食われる退行が緑のまま通る（helper_test.go の decideVia の doc）。
// HandlesBack のクロージャを nil にすると、この 2 つが赤くなる。
func TestFormEscIsHandledByFormNotOverlay(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		typed string // esc の前に打つ文字。空なら未入力のまま esc を打つ
		want  tea.Msg
	}{
		"未入力なら中断":  {typed: "", want: dialog.FormAbortedMsg{}},
		"入力済みなら破棄": {typed: "x", want: dialog.FormDiscardMsg{}},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			o, st := newFormOverlay(t)
			var th huh.Theme
			OpenForm(&o, st, formTitleText, probeForm(&th))
			if tt.typed != "" {
				o.Update(pagetest.Press(tt.typed))
			}

			res := decideVia(t, &o, "esc")

			if !o.Active() {
				t.Error("esc で Overlay がフォームを閉じた（破棄の確認を出せない）")
			}
			if res.Kind != FormKind {
				t.Errorf("esc の決定の種類 = %q, want %q", res.Kind, FormKind)
			}
			if got, want := res.Msg, tt.want; got != want {
				t.Errorf("esc の決定の中身 = %T(%v), want %T(%v)", got, got, want, want)
			}
		})
	}
}

// 開く指示に載せた見出しがそのまま表示される。
func TestOpenFormShowsGivenTitle(t *testing.T) {
	t.Parallel()

	o, st := newFormOverlay(t)
	var th huh.Theme
	OpenForm(&o, st, formTitleText, probeForm(&th))

	if got := formTitle(modelOf(t, o, FormKind)); got != formTitleText {
		t.Errorf("見出し = %q, want %q", got, formTitleText)
	}
	if !strings.Contains(o.View(), "名前") {
		t.Error("組み立て関数が返したフォームが表示されていない")
	}
	if got := formHints(modelOf(t, o, FormKind)); len(got) == 0 {
		t.Error("フッタのキーヒントが空である")
	}
}

// 組み立て関数へ渡すテーマは、登録時ではなく**開くときに渡された共有状態**から組む。
//
// **見ているのは組み立て関数へ渡された引数だけである——フォームが最終的に着る配色は
// ここでは観測していない。** dialog.Form.SetForm の後に dialog.Form 自身が
// token.HuhTheme(f.styles, f.color) でテーマを上書きするので、formModal.open が組んで
// 渡したテーマは捨てられる。つまり **open は msg.st を内側の dialog.Form へ直接は
// 渡さない**（Restyle も呼ばない）。
//
// **同じ色は別の経路で内側へ届く。** page.Overlay.Open は formOpenMsg を配る前に最新の
// 共有状態を配り直し（overlay.go の replay）、その page.StateMsg で f.styles / f.color が
// 最新になってから SetForm と上書きが走る（overlaystate.go の send は Update を同期で
// 呼ぶ）。**したがって「開くときの配色が内側へ伝わらない」という症状は起きない。**
// ここで縛るのは**配線**——開くときに渡した共有状態が組み立て関数まで届くこと——に
// 限る、というだけである。
//
// **色の有無は見出しの太字で見る。** token.HuhTheme が色を使うときだけ
// `Focused.Title` へ `Foreground(accent).Bold(true)` を与えるためである
// （token/huhtheme.go の paintStyles）。前景色そのものでは見分けられない——
// lipgloss の GetForeground は未設定でもゼロ値の色を返すので、nil 比較は
// どちらでも真になる。
func TestOpenFormPassesThemeBuiltFromGivenState(t *testing.T) {
	t.Parallel()

	tests := map[string]bool{"色あり": true, "色なし": false}

	for name, color := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			o, st := newFormOverlay(t)
			opening := st
			opening.Color = color

			var th huh.Theme
			OpenForm(&o, opening, formTitleText, probeForm(&th))

			if th == nil {
				t.Fatal("組み立て関数が呼ばれていない")
			}
			painted := th.Theme(opening.Dark).Focused.Title.GetBold()
			if painted != color {
				t.Errorf("組み立て関数へ渡されたテーマの着色 = %v, want %v"+
					"（開くときの共有状態が効いていない）", painted, color)
			}
			// 期待する側も token.HuhTheme で組む（この判別が色で分かれることの陽性対照。
			// 判別できない欄を選ぶと、上の比較は両方の枝で同じ値になって素通りする）。
			want := token.HuhTheme(opening.Styles, color).Theme(opening.Dark).Focused.Title.GetBold()
			if painted != want {
				t.Errorf("組み立て関数へ渡されたテーマの着色 = %v, want %v", painted, want)
			}
		})
	}
}

// 見出しとフッタは他のモーダルの Model を渡されても落ちない。
func TestFormTitleAndHintsIgnoreForeignModel(t *testing.T) {
	t.Parallel()

	foreign := pagetest.NewEcho()
	if got := formTitle(foreign); got != "" {
		t.Errorf("別のモーダルの見出し = %q, want 空", got)
	}
	if got := formHints(foreign); got != nil {
		t.Errorf("別のモーダルのフッタ = %v, want nil", got)
	}
}
