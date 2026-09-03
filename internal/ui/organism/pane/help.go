package pane

import (
	"strings"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/ui/keymap"
	"github.com/ousiassllc/gsr-helper/internal/ui/token"
)

// Help は ? の全キー一覧。列組みと折り返しは bubbles/help に委ねる。
//
// フッタ（chromebar.KeyBar）には使わない。bubbles/help は無効な Binding をキーごと
// 非表示にする設計であり、「キーを消さずグレーアウトして理由を示す」と両立しない
// （atomic-design.md の Help と bubbles/help）。
type Help struct {
	h      help.Model
	keys   keymap.List
	groups [][]key.Binding
	offset int // スクロール位置（先頭から隠す行数）
	height int
}

// NewHelp は全キー一覧を組み立てる。groups は画面が自分の範囲で選んだキーのグループ
// （keymap.Set.Help の結果）を渡す。
//
// スクロールのキー定義を受け取るのは、キー数が高さを超えたときに続きへ辿れるように
// するためである。**受け取らないと切り詰めた行に到達する手段が無くなる**（モーダルの
// 最上位に居る間はキーが背後へ流れない。page/overlay.go）。
//
// 配色は token.Styles から組み立てる。bubbles/help の既定スタイルに任せると、色を無効に
// した設定でも lipgloss のカラープロファイル判定で装飾が入り、色の可否を決める箇所が
// 2 つになる（atomic-design.md の背景の明暗と NO_COLOR）。
func NewHelp(s token.Styles, groups [][]key.Binding, keys keymap.List) Help {
	m := help.New()
	m.Styles = helpStyles(s)
	return Help{h: m, keys: keys, groups: alwaysEnabled(groups), offset: 0, height: 0}
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
//
// 幅が変わると bubbles/help の列組みが変わって行数も変わるため、スクロール位置を
// 収まる範囲へ丸め直す。丸めないと「何も出ない空のヘルプ」になる。
func (m *Help) SetSize(w, h int) {
	m.h.SetWidth(w)
	m.height = h
	m.offset = min(m.offset, m.maxOffset())
}

// Offset はスクロール位置を返す。
func (m Help) Offset() int { return m.offset }

// SetOffset はスクロール位置を設定する。配色やキー定義の差し替えで作り直したときに
// 位置を引き継ぐために使う。
func (m *Help) SetOffset(n int) {
	m.offset = min(max(n, 0), m.maxOffset())
}

// Update はスクロールのキーを処理する。
//
// 一覧と同じキー（j / k / ctrl+f / ctrl+b）を使う。ヘルプだけ別のキーにすると、
// 「どの画面でも同じキーで動く」という約束から外れる。
func (m Help) Update(msg tea.Msg) (Help, tea.Cmd) {
	press, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return m, nil
	}

	switch {
	case key.Matches(press, m.keys.Down):
		m.SetOffset(m.offset + 1)
	case key.Matches(press, m.keys.Up):
		m.SetOffset(m.offset - 1)
	case key.Matches(press, m.keys.PageDown):
		m.SetOffset(m.offset + max(m.height, 1))
	case key.Matches(press, m.keys.PageUp):
		m.SetOffset(m.offset - max(m.height, 1))
	case key.Matches(press, m.keys.Top):
		m.SetOffset(0)
	case key.Matches(press, m.keys.Bottom):
		m.SetOffset(m.maxOffset())
	}
	return m, nil
}

// Scrollable は高さに収まらず、スクロールが必要かを返す。フッタのキーヒントを
// 出すかの判断に使う。
func (m Help) Scrollable() bool { return m.maxOffset() > 0 }

// View は全キーの一覧のうち、スクロール位置から高さ分を返す。
func (m Help) View() string {
	lines := m.lines()
	if m.height <= 0 || len(lines) <= m.height {
		return strings.Join(lines, "\n")
	}

	off := min(m.offset, m.maxOffset())
	return strings.Join(lines[off:off+m.height], "\n")
}

// lines は全キー一覧の全行を返す。
func (m Help) lines() []string {
	return strings.Split(m.h.FullHelpView(m.groups), "\n")
}

// maxOffset はスクロールできる最大位置を返す。
func (m Help) maxOffset() int {
	if m.height <= 0 {
		return 0
	}
	return max(len(m.lines())-m.height, 0)
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
