package setup

import (
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/ui/atom"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
)

// chrome は状態行とフッタを親へ送る Cmd を返す。
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

// input は入力中の表示を返す。
//
// フォームの入力中はグローバルキーが効かない。何も出さないと画面が無反応に
// なったように見える（screens.md「入力中（絞り込み・フィルタ・フォーム）」）。
func (m Model) input() string {
	if m.formShown {
		return "フォーム"
	}
	return ""
}

// status は状態行の本文を返す。
//
// 優先順は 入力中 > 直近の案内 > 準備中 > 実行中 > 直近の結果 とする
// （page/disk の status と同じ考え方で、いま起きていることを先に出す）。
func (m Model) status() string {
	switch {
	case m.notice != "":
		return m.notice
	case m.waiting:
		return "実行内容を確認しています…"
	case m.run != nil:
		return m.run.title
	case len(m.report) > 0:
		return m.report[0]
	default:
		return ""
	}
}

// footer はフッタ 1 行目のキーヒントを返す。
func (m Model) footer() []atom.Hint {
	if m.overlay.Active() {
		return m.overlay.Hints()
	}

	keys := m.st.Keys
	addOK, addWhy := m.allowed(page.BindingKey(keys.Runner.Add))
	upOK, upWhy := m.allowed(page.BindingKey(keys.Runner.Update))

	return []atom.Hint{
		{
			Key: page.BindingKey(keys.List.Enter), Desc: "実行",
			Enabled: true, Reason: "",
		},
		{
			Key: page.BindingKey(keys.Runner.Add), Desc: "追加",
			Enabled: addOK, Reason: addWhy,
		},
		{
			Key: page.BindingKey(keys.Runner.Update), Desc: "更新",
			Enabled: upOK, Reason: upWhy,
		},
	}
}

// runTitle は進捗の見出しを組み立てる（screens.md の `追加中… 2/3`）。
func runTitle(kind string, done, total int) string {
	return kind + "中… " + strconv.Itoa(done) + "/" + strconv.Itoa(total)
}

// reportView は結果報告を本文の下に描く。
func (m Model) reportView() string {
	lines := make([]string, 0, len(m.report))
	for _, l := range m.report {
		lines = append(lines, atom.Truncate(l, m.st.BodyW))
	}
	return m.st.Styles.Muted.Render(strings.Join(lines, "\n"))
}
