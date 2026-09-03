package pane

import (
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/ui/token"
)

// Log が持つフィルタ入力欄の組み立てと出し入れを集める（1 ファイル 300 行の上限に
// 収めるための分割。log.go と同じパッケージである）。

// filterPrompt はフィルタ行の見出し。入力中と確定後で同じ文字列を使う。
const filterPrompt = "フィルタ: "

// newFilterInput はフィルタの入力欄を組み立てる。
func newFilterInput(s token.Styles) textinput.Model {
	in := textinput.New()
	in.Prompt = filterPrompt
	in.SetStyles(filterStyles(s))
	return in
}

// filterStyles は token のスタイルを bubbles/textinput のスタイルへ写す。
//
// 既定スタイルに任せない理由は helpStyles と同じで、色を無効にした設定でも lipgloss の
// カラープロファイル判定で装飾が入り、色の可否を決める箇所が 2 つになるためである。
//
// organism/table にも同じ写し替えがあるが、共有しない。organism と organism/pane は
// どちらの向きにも import しない規約であり（atomic-design.md の依存の規則）、共有の
// ためだけに向きを作ると分割の目的（行数上限の分散）が崩れる。
func filterStyles(s token.Styles) textinput.Styles {
	st := textinput.StyleState{
		Text:        s.Style(token.RolePlain),
		Placeholder: s.Muted,
		Suggestion:  s.Muted,
		Prompt:      s.Muted,
	}
	return textinput.Styles{
		Focused: st,
		Blurred: st,
		Cursor: textinput.CursorStyle{
			Color: s.Cursor.GetForeground(), Shape: tea.CursorBlock,
			Blink: true, BlinkSpeed: 0, // 0 は bubbles の既定（約 500ms）
		},
	}
}

// Filtering は入力モードかを返す。page は状態行の「入力中」に使う。
func (l Log) Filtering() bool { return l.filtering }

// Filter は確定済みのフィルタを返す。page はこれを正規表現に解いて行を絞る。
func (l Log) Filter() string { return l.applied }

// FilterView は見出しに出すフィルタの表記を返す。
//
// 入力中は入力欄そのもの（カーソルを含む）を、確定後は確定値を返す。フィルタが
// 空で入力中でもなければ空文字を返し、page が見出しから項目ごと落とせるようにする。
func (l Log) FilterView() string {
	switch {
	case l.filtering:
		return l.filter.View()
	case l.applied == "":
		return ""
	default:
		return filterPrompt + l.applied
	}
}

// StartFilter は入力モードへ入る。返る Cmd はカーソルの点滅であり、捨てると
// カーソルが出ない。
func (l *Log) StartFilter() tea.Cmd {
	l.filtering = true
	return l.filter.Focus()
}

// AcceptFilter は入力中の値を確定する。
func (l *Log) AcceptFilter() {
	l.applied = l.filter.Value()
	l.stopFilter()
}

// CancelFilter は入力を取り消し、確定済みの値へ戻す。
func (l *Log) CancelFilter() {
	l.filter.SetValue(l.applied)
	l.stopFilter()
}

// ClearFilter は確定済みのフィルタを解除する（入力中でないときの esc）。
func (l *Log) ClearFilter() {
	l.applied = ""
	l.filter.SetValue("")
	l.stopFilter()
}

// stopFilter は入力モードを抜ける。
func (l *Log) stopFilter() {
	l.filtering = false
	l.filter.Blur()
}
