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

// input は入力中の表示を返す。フォームの入力中はグローバルキーが効かない。
func (m Model) input() string {
	if m.formShown {
		return "フォーム"
	}
	return ""
}

// status は状態行の本文を返す。
// 優先順は 案内 > 実行中 > 直近の結果（page/setup と同じ考え方）。
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

// formTitleOf はフォームの見出しを返す。
func formTitleOf(k kind) string {
	switch k {
	case kindEnv:
		return ".env の編集"
	case kindPath:
		return ".path の編集"
	case kindDropIn:
		return "systemd drop-in の編集"
	case kindLabels:
		return "ラベルの編集"
	case kindGroup:
		return "runner group の変更"
	case kindCopy:
		return ".env を他の runner へ複製"
	case kindReregister:
		return ""
	default:
		return ""
	}
}

// title は差分の見出しに出す対象を返す。
func (c change) title() string {
	if c.path != "" {
		return c.path
	}

	switch c.kind {
	case kindLabels:
		return "ラベル（GitHub）"
	case kindGroup:
		return "runner group（GitHub）"
	case kindCopy:
		return ".env の複製（" + itoa(len(c.copies)) + " 台）"
	case kindEnv, kindPath, kindDropIn, kindReregister:
		return ""
	default:
		return ""
	}
}

// itoa は小さな整数を文字列にする。strconv を 1 か所のために import しない。
func itoa(n int) string {
	if n == 0 {
		return "0"
	}

	var b []byte
	for ; n > 0; n /= 10 {
		b = append([]byte{byte('0' + n%10)}, b...)
	}
	return string(b)
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

// validateLabelList はフォームの入力を検証して整えたラベルを返す（FR-36）。
func validateLabelList(s string) ([]string, error) {
	return config.ValidateLabels(splitLabels(s))
}
