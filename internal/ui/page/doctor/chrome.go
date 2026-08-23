package doctor

import (
	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/ui/atom"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
)

// 親へ返す本体以外の状態（状態行とフッタ）の組み立てをここに集める。
// 一覧の操作（doctor.go）と分けてあるのは 1 ファイルの行数を抑えるためである。

// chrome は親へ本体以外の状態を知らせる Cmd を返す。
func (m Model) chrome() tea.Cmd {
	c := page.ChromeMsg{
		Tab:    m.tab,
		Modal:  m.overlay.Active(),
		Input:  m.input(),
		Status: m.status(),
		Footer: m.footer(),
	}
	return func() tea.Msg { return c }
}

// input は入力中の名称を返す。入力中でなければ空文字を返す。
func (m Model) input() string {
	if m.tbl.Filtering() {
		return inputFilter
	}
	return ""
}

// status は状態行に出す page 側の文を返す。
//
// 優先順は 入力中 → 実行中 → 絞り込みの件数。入力中はグローバルキーが効かない
// 状態そのものなので最優先で示す（screens.md の状態行）。
func (m Model) status() string {
	if in := m.input(); in != "" {
		return "入力中: " + in
	}
	if m.running {
		return runningStatus
	}
	if q := m.tbl.FilterValue(); q != "" {
		return "絞り込み: " + q
	}
	return ""
}

// footer はフッタのキーヒントを返す。
func (m Model) footer() []atom.Hint {
	if m.overlay.Active() {
		return m.overlay.Hints()
	}
	_, hasRow := m.tbl.Selected()
	return []atom.Hint{
		{
			Key:     page.BindingKey(m.st.Keys.List.Enter),
			Desc:    detailDesc,
			Enabled: hasRow,
			Reason:  "",
		},
		{
			Key:     page.BindingKey(m.st.Keys.Global.Refresh),
			Desc:    rerunDesc,
			Enabled: !m.running,
			Reason:  runningReason(m.running),
		},
	}
}

// runningReason は実行中に再実行が使えない理由を返す。
func runningReason(running bool) string {
	if running {
		return runningStatus
	}
	return ""
}
