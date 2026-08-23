package token

import (
	"image/color"
	"strings"
	"testing"

	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"
)

// 色を使わない指定のときは、どの装飾にも色が残らない。
//
// huh.ThemeBase はボタンの前景・背景とプレースホルダとヘルプに色を直に置く。
// 落とし漏れがあると、NO_COLOR を指定した端末でフォームだけが色付きで残る。
func TestHuhThemeNoColorLeavesNoANSI(t *testing.T) {
	st := HuhTheme(NewStyles(true, false), false).Theme(true)

	for name, s := range huhColorTargets(st) {
		if colored(s.GetForeground()) {
			t.Errorf("%s に前景色が残っている", name)
		}
		if colored(s.GetBackground()) {
			t.Errorf("%s に背景色が残っている", name)
		}
	}
}

// 色付きの Styles を渡しても、色を使わない指定が勝つ。
//
// 色の有効無効を Styles と別に受け取るのは、NO_COLOR / --no-color / 非 TTY の判定を
// cmd で 1 つに決めて配るためである。Styles の側が色付きのまま届いても縮退させる。
func TestHuhThemeNoColorBeatsStyles(t *testing.T) {
	st := HuhTheme(NewStyles(true, true), false).Theme(true)

	if colored(st.Focused.Title.GetForeground()) {
		t.Error("色を使わない指定なのに見出しに色が付いている")
	}
	if got := st.Focused.Title.Render("追加"); strings.Contains(got, "\x1b") {
		t.Errorf("見出しの描画に ANSI 列が混ざっている: %q", got)
	}
}

// 色を使う指定のときは Styles の色がそのまま写る。
//
// 同じ意味の色が画面によって変わると、フォームだけ別のツールに見える
// （atomic-design.md の「`Form` と huh」）。
func TestHuhThemeUsesStyles(t *testing.T) {
	s := NewStyles(true, true)
	st := HuhTheme(s, true).Theme(true)

	for _, tc := range []struct {
		name  string
		got   lipgloss.Style
		want  lipgloss.Style
		label string
	}{
		{name: "見出し", got: st.Focused.Title, want: s.Accent, label: "Accent"},
		{name: "補足", got: st.Focused.Description, want: s.Muted, label: "Muted"},
		{name: "検証エラー", got: st.Focused.ErrorMessage, want: s.Fail, label: "Fail"},
		{name: "選択済み", got: st.Focused.SelectedOption, want: s.OK, label: "OK"},
		{name: "選択の記号", got: st.Focused.SelectSelector, want: s.Accent, label: "Accent"},
		{name: "入力欄のカーソル", got: st.Focused.TextInput.Cursor, want: s.Accent, label: "Accent"},
		{name: "キーヒントのキー", got: st.Help.ShortKey, want: s.Accent, label: "Accent"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.got.GetForeground() != tc.want.GetForeground() {
				t.Errorf("%s の色が Styles.%s と違う", tc.name, tc.label)
			}
		})
	}

	// 区画の見出しは項目の見出しと同じ色にする（見出しの色が 2 通りにならないように）。
	if st.Group.Title.GetForeground() != st.Focused.Title.GetForeground() {
		t.Error("区画の見出しと項目の見出しで色が違う")
	}
}

// 背景の明暗は渡されても使わない。
//
// Styles は cmd が解決済みの値であり、ここで明暗を読み直すと同じ画面に 2 つの判定が
// 並ぶ。huh が描画のたびに渡してくる isDark は捨てる。
func TestHuhThemeIgnoresIsDark(t *testing.T) {
	theme := HuhTheme(NewStyles(true, true), true)

	dark, light := theme.Theme(true), theme.Theme(false)
	if dark.Focused.Title.GetForeground() != light.Focused.Title.GetForeground() {
		t.Error("isDark でテーマの色が変わっている")
	}
}

// 選択中のボタンは色を使わなくても見分けられる。
//
// 反転（Reverse）は色ではないため NO_COLOR でも残せる装飾であり、色を使えない端末で
// 「どちらのボタンに居るのか」を示す唯一の手がかりになる（screens.md の設計原則 4）。
func TestHuhThemeFocusedButtonIsReversed(t *testing.T) {
	for _, color := range []bool{true, false} {
		st := HuhTheme(NewStyles(true, color), color).Theme(true)
		if !st.Focused.FocusedButton.GetReverse() {
			t.Errorf("色 %v: 選択中のボタンが反転していない", color)
		}
		if st.Focused.BlurredButton.GetReverse() {
			t.Errorf("色 %v: 選択していないボタンまで反転している", color)
		}
	}
}

// colored は装飾に色が設定されているかを返す。
//
// lipgloss は色が未設定の装飾に nil ではなく NoColor{} を返す（Style.GetForeground の
// 契約）。nil との比較だけでは「色を落とした」ことを検証できない。
func colored(c color.Color) bool {
	if c == nil {
		return false
	}
	_, none := c.(lipgloss.NoColor)
	return !none
}

// huhColorTargets は huh.ThemeBase が色を置く装飾を名前付きで返す。
//
// ThemeBase が色を持つのはボタン 2 つ・プレースホルダ・ヘルプだけである。ここに
// 挙げていない装飾は枠と記号の構造しか持たない。
func huhColorTargets(st *huh.Styles) map[string]lipgloss.Style {
	targets := map[string]lipgloss.Style{
		"ヘルプの中略":       st.Help.Ellipsis,
		"ヘルプのキー":       st.Help.ShortKey,
		"ヘルプの説明":       st.Help.ShortDesc,
		"ヘルプの区切り":      st.Help.ShortSeparator,
		"ヘルプのキー（全一覧）":  st.Help.FullKey,
		"ヘルプの説明（全一覧）":  st.Help.FullDesc,
		"ヘルプの区切り（全一覧）": st.Help.FullSeparator,
	}
	for name, fs := range map[string]huh.FieldStyles{"焦点あり": st.Focused, "焦点なし": st.Blurred} {
		targets[name+"の選択中のボタン"] = fs.FocusedButton
		targets[name+"の選択していないボタン"] = fs.BlurredButton
		targets[name+"のプレースホルダ"] = fs.TextInput.Placeholder
	}
	return targets
}
