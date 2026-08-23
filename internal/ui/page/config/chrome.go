package config

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/config"
	"github.com/ousiassllc/gsr-helper/internal/runner"
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

// input は入力中の表示を返す。入力中はグローバルキーが効かない。
func (m Model) input() string {
	if m.formShown {
		return "フォーム"
	}
	return ""
}

// status は状態行の本文を返す。優先順は 案内 > 実行中 > 直近の結果。
func (m Model) status() string {
	switch {
	case m.notice != "":
		return m.notice
	case m.busy:
		return "設定を処理しています…"
	default:
		return m.report
	}
}

// footer はフッタ 1 行目のキーヒントを返す。
func (m Model) footer() []atom.Hint {
	if m.overlay.Active() {
		return m.overlay.Hints()
	}

	desc := "編集"
	if m.target.Dir == "" {
		desc = "選択"
	}

	return []atom.Hint{
		{Key: page.BindingKey(m.st.Keys.List.Enter), Desc: desc, Enabled: true, Reason: ""},
		{Key: page.BindingKey(m.st.Keys.Global.Back), Desc: "戻る", Enabled: true, Reason: ""},
	}
}

// names は runner の名前を並べて返す。
func names(rs []runner.Runner) []string {
	out := make([]string, 0, len(rs))
	for _, r := range rs {
		out = append(out, r.Name())
	}
	return out
}

// joinLabels はラベルの並びをフォームの 1 行にする。
func joinLabels(labels []string) string { return strings.Join(labels, ",") }

// validateLabelList は入力を検証して整えたラベルを返す（FR-36）。
func validateLabelList(s string) ([]string, error) { return config.ValidateLabels(splitLabels(s)) }
