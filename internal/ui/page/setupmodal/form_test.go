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
// **判断は 2 つある。** 完了・中断・破棄の 3 つをどれも同じ種類（FormKind）で
// 差し戻すこと（page 側が 1 つの case で受けられるのはそのためである）と、huh の
// テーマを**開くときに渡された共有状態**から組むことである。後者を登録時の値で
// 組むと、起動後に配色が変わったフォームが古い配色で開く。

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

// 完了・中断・破棄はどれも FormKind の決定として差し戻される。
//
// 3 つのうち 1 つでも別の種類（あるいは包み忘れ）になると、page はその結末を
// 受け取れず、**フォームが閉じないまま残る**。
func TestFormResultCarriesFormKindForEveryOutcome(t *testing.T) {
	t.Parallel()

	tests := map[string]tea.Msg{
		"完了": dialog.FormDoneMsg{},
		"中断": dialog.FormAbortedMsg{},
		"破棄": dialog.FormDiscardMsg{},
	}

	for name, msg := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			o, st := newFormOverlay(t)
			var th huh.Theme
			OpenForm(&o, st, formTitleText, probeForm(&th))

			_, cmd := o.Update(msg)

			res := resultOf(t, cmd)
			if res.Kind != FormKind {
				t.Errorf("決定の種類 = %q, want %q", res.Kind, FormKind)
			}
			if got, want := res.Msg, msg; got != want {
				t.Errorf("決定の中身 = %T(%v), want %T(%v)", got, got, want, want)
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

// テーマは登録時ではなく**開くときに渡された共有状態**から組む。
//
// 登録時の色で組むと、起動後に配色が変わってから開いたフォームが古い配色で描かれる
// （formModal.open が msg.st を見る理由）。
//
// **色の有無は見出しの太字で見る。** token.HuhTheme が色を使うときだけ
// `Focused.Title` へ `Foreground(accent).Bold(true)` を与えるためである
// （token/huhtheme.go の paintStyles）。前景色そのものでは見分けられない——
// lipgloss の GetForeground は未設定でもゼロ値の色を返すので、nil 比較は
// どちらでも真になる。
func TestOpenFormUsesColorFromGivenState(t *testing.T) {
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
				t.Errorf("テーマの着色 = %v, want %v（開くときの共有状態が効いていない）", painted, color)
			}
			// 期待する側も token.HuhTheme で組む（この判別が色で分かれることの陽性対照。
			// 判別できない欄を選ぶと、上の比較は両方の枝で同じ値になって素通りする）。
			want := token.HuhTheme(opening.Styles, color).Theme(opening.Dark).Focused.Title.GetBold()
			if painted != want {
				t.Errorf("テーマの着色 = %v, want %v", painted, want)
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
