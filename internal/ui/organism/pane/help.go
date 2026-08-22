package pane

import (
	"strings"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"

	"github.com/ousiassllc/gsr-helper/internal/ui/token"
)

// Help は ? の全キー一覧。列組みと折り返しは bubbles/help に委ねる。
//
// フッタ（molecule.KeyBar）には使わない。bubbles/help は無効な Binding をキーごと
// 非表示にする設計であり、「キーを消さずグレーアウトして理由を示す」と両立しない
// （atomic-design.md の Help と bubbles/help）。
type Help struct {
	h      help.Model
	groups [][]key.Binding
	height int
}

// NewHelp は全キー一覧を組み立てる。groups は keymap.Set.FullHelp の結果を渡す。
//
// 配色は token.Styles から組み立てる。bubbles/help の既定スタイルに任せると、色を無効に
// した設定でも lipgloss のカラープロファイル判定で装飾が入り、色の可否を決める箇所が
// 2 つになる（atomic-design.md の背景の明暗と NO_COLOR）。
func NewHelp(s token.Styles, groups [][]key.Binding) Help {
	m := help.New()
	m.Styles = helpStyles(s)
	return Help{h: m, groups: alwaysEnabled(groups), height: 0}
}

// helpStyles は token のスタイルを bubbles/help のスタイルへ写す。
//
// キーはフッタ（atom.KeyHint）と同じ強調、説明・区切り・中略記号は補足情報として薄く描く。
func helpStyles(s token.Styles) help.Styles {
	return help.Styles{
		Ellipsis:       s.Muted,
		ShortKey:       s.Accent,
		ShortDesc:      s.Muted,
		ShortSeparator: s.Muted,
		FullKey:        s.Accent,
		FullDesc:       s.Muted,
		FullSeparator:  s.Muted,
	}
}

// SetSize はヘルプに割り当てられた領域を設定する。
func (m *Help) SetSize(w, h int) {
	m.h.SetWidth(w)
	m.height = h
}

// View は全キーの一覧を返す。
func (m Help) View() string {
	out := m.h.FullHelpView(m.groups)
	if m.height <= 0 {
		return out
	}

	lines := strings.Split(out, "\n")
	if len(lines) <= m.height {
		return out
	}
	return strings.Join(lines[:m.height], "\n")
}

// alwaysEnabled は各 Binding を有効にした写しを返す。
//
// 全キー一覧は操作の可否を反映しない（screens.md の無効な操作の表示）。bubbles/help は
// 無効な Binding を描かないので、ここで有効化してから渡す。
func alwaysEnabled(groups [][]key.Binding) [][]key.Binding {
	out := make([][]key.Binding, 0, len(groups))
	for _, g := range groups {
		group := make([]key.Binding, 0, len(g))
		for _, b := range g {
			b.SetEnabled(true)
			group = append(group, b)
		}
		out = append(out, group)
	}
	return out
}
