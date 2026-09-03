package token

import (
	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"
)

// このファイルだけは lipgloss に加えて huh を import する。
//
// token は「lipgloss のみを import する最下層」だが（atomic-design.md の階層の規則）、
// huh.Theme の組み立ては token の中に置くと定めてある（同「`Form` と huh」）。色の定義を
// token の外に作らないことを優先した結果の例外であり、**この 1 ファイルに閉じる**。
// 他のファイルから huh を参照しないこと。

// HuhTheme は解決済みの Styles から huh のテーマを組み立てる。
//
// フォームだけ配色が浮くことを防ぐためであり、NO_COLOR / --no-color / 非 TTY の
// 縮退も Styles と同じ経路で伝わる（atomic-design.md の「`Form` と huh」）。
//
// **背景の明暗は引数に取らない。** huh.Theme は描画のたびに isDark を渡してくるが、
// Styles は cmd が解決済みの値であり、ここで明暗を読み直すと同じ画面に 2 つの
// 判定が並ぶ。渡された isDark は捨てて、常に Styles の色を返す。
//
// color が false のときは Styles の色を一切参照せず、huh.ThemeBase が自前で置いた色
// （ボタンの前景・背景、プレースホルダ、ヘルプ）も落とした素のテーマを返す。Styles が
// 色付きのまま渡されても NO_COLOR の指定が勝つようにするためである。
func HuhTheme(s Styles, color bool) huh.Theme {
	return huh.ThemeFunc(func(bool) *huh.Styles {
		t := huh.ThemeBase(true)
		clearBaseColors(t)
		if color {
			paintStyles(t, s)
		}
		return t
	})
}

// clearBaseColors は huh.ThemeBase が直に置いた色を落とす。
//
// ThemeBase が色を持つのはボタン 2 つ・プレースホルダ・ヘルプだけであり、残りは
// 枠と記号（`> ` / `[ ] ` など）の構造だけを持つ。落とし漏れがあると NO_COLOR でも
// その部分だけ色が出る。
//
// **落とすのと同時に、選択中のボタンへ反転（Reverse）を与える。** 反転は色ではないため
// NO_COLOR でも残せる装飾であり、色を使えない端末で「どちらのボタンに居るのか」を
// 示す唯一の手がかりになる（screens.md の設計原則 4）。
func clearBaseColors(t *huh.Styles) {
	for _, f := range []*huh.FieldStyles{&t.Focused, &t.Blurred} {
		f.FocusedButton = clearColor(f.FocusedButton).Reverse(true)
		f.BlurredButton = clearColor(f.BlurredButton)
		f.TextInput.Placeholder = clearColor(f.TextInput.Placeholder)
	}

	t.Help.Ellipsis = clearColor(t.Help.Ellipsis)
	t.Help.ShortKey = clearColor(t.Help.ShortKey)
	t.Help.ShortDesc = clearColor(t.Help.ShortDesc)
	t.Help.ShortSeparator = clearColor(t.Help.ShortSeparator)
	t.Help.FullKey = clearColor(t.Help.FullKey)
	t.Help.FullDesc = clearColor(t.Help.FullDesc)
	t.Help.FullSeparator = clearColor(t.Help.FullSeparator)
}

// clearColor は装飾から色だけを落とす。字下げ・余白・枠・添え字（SetString）は残す。
//
// lipgloss.NewStyle() で置き換えないのは、huh が記号（`> ` / `[x] `）や余白を装飾に
// 埋め込んでおり、丸ごと捨てるとフォームの体裁が崩れるためである。
func clearColor(st lipgloss.Style) lipgloss.Style {
	return st.UnsetForeground().UnsetBackground().UnsetBorderForeground().UnsetBorderBackground()
}

// paintStyles は Styles の色を huh のテーマへ写す。
//
// 対応づけは他の画面と揃える。見出しとカーソル相当（選択の記号・入力欄のプロンプト）は
// Accent、補足は Muted、検証エラーは Fail、選択済みは OK である。同じ意味の色が画面に
// よって変わると、フォームだけ別のツールに見える。
func paintStyles(t *huh.Styles, s Styles) {
	accent := s.Accent.GetForeground()
	muted := s.Muted.GetForeground()

	t.Focused.Base = t.Focused.Base.BorderForeground(s.Divider.GetForeground())
	t.Focused.Card = t.Focused.Base
	t.Focused.Title = t.Focused.Title.Foreground(accent).Bold(true)
	t.Focused.NoteTitle = t.Focused.NoteTitle.Foreground(accent).Bold(true)
	t.Focused.Description = t.Focused.Description.Foreground(muted)
	t.Focused.ErrorIndicator = t.Focused.ErrorIndicator.Foreground(s.Fail.GetForeground())
	t.Focused.ErrorMessage = t.Focused.ErrorMessage.Foreground(s.Fail.GetForeground())
	t.Focused.SelectSelector = t.Focused.SelectSelector.Foreground(accent)
	t.Focused.MultiSelectSelector = t.Focused.MultiSelectSelector.Foreground(accent)
	t.Focused.NextIndicator = t.Focused.NextIndicator.Foreground(accent)
	t.Focused.PrevIndicator = t.Focused.PrevIndicator.Foreground(accent)
	t.Focused.SelectedOption = t.Focused.SelectedOption.Foreground(s.OK.GetForeground())
	t.Focused.SelectedPrefix = t.Focused.SelectedPrefix.Foreground(s.OK.GetForeground())
	t.Focused.UnselectedPrefix = t.Focused.UnselectedPrefix.Foreground(muted)
	t.Focused.Directory = t.Focused.Directory.Foreground(accent)
	t.Focused.TextInput.Cursor = t.Focused.TextInput.Cursor.Foreground(accent)
	t.Focused.TextInput.Prompt = t.Focused.TextInput.Prompt.Foreground(accent)
	t.Focused.TextInput.Placeholder = t.Focused.TextInput.Placeholder.Foreground(s.Skip.GetForeground())
	// 反転は clearBaseColors が既に与えてある。前景に Accent を置くと、反転の結果
	// Accent が背景になり、文字は端末の地の色で読める。
	t.Focused.FocusedButton = t.Focused.FocusedButton.Foreground(accent)
	t.Focused.BlurredButton = t.Focused.BlurredButton.Foreground(muted)
	t.Focused.Next = t.Focused.FocusedButton

	blurStyles(t)
	helpStyles(t, s)

	t.Group.Title = t.Focused.Title
	t.Group.Description = t.Focused.Description
}

// blurStyles は焦点の当たっていない側を、色だけ揃えて構造は huh.ThemeBase に戻す。
//
// ThemeBase と同じく Focused から写して枠と記号を差し替える。写さずに個別へ色を置くと、
// 色の対応づけが 2 か所に分かれて片方だけが古くなる。
func blurStyles(t *huh.Styles) {
	t.Blurred = t.Focused
	t.Blurred.Base = t.Focused.Base.BorderStyle(lipgloss.HiddenBorder())
	t.Blurred.Card = t.Blurred.Base
	t.Blurred.MultiSelectSelector = lipgloss.NewStyle().SetString("  ")
	t.Blurred.NextIndicator = lipgloss.NewStyle()
	t.Blurred.PrevIndicator = lipgloss.NewStyle()
}

// helpStyles はフォームが自分で描くキーヒントの色を揃える。
//
// キーを Accent、説明と区切りを Muted にするのは atom.KeyHint と同じ配分である。
// フッタ（chromebar.KeyBar）と並んで見えるため、ここだけ配色が違うと同じキーヒントが
// 2 通りの見え方で並ぶ。
func helpStyles(t *huh.Styles, s Styles) {
	accent := s.Accent.GetForeground()
	muted := s.Muted.GetForeground()

	t.Help.ShortKey = t.Help.ShortKey.Foreground(accent)
	t.Help.FullKey = t.Help.FullKey.Foreground(accent)
	t.Help.ShortDesc = t.Help.ShortDesc.Foreground(muted)
	t.Help.FullDesc = t.Help.FullDesc.Foreground(muted)
	t.Help.ShortSeparator = t.Help.ShortSeparator.Foreground(muted)
	t.Help.FullSeparator = t.Help.FullSeparator.Foreground(muted)
	t.Help.Ellipsis = t.Help.Ellipsis.Foreground(muted)
}
