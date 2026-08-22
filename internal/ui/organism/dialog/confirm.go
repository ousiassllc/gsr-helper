package dialog

import (
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/ousiassllc/gsr-helper/internal/ui/atom"
	"github.com/ousiassllc/gsr-helper/internal/ui/keymap"
	"github.com/ousiassllc/gsr-helper/internal/ui/token"
)

const (
	// headingTargets は対象の一覧に付ける見出し。
	headingTargets = "対象:"
	// headingCommand は実行するコマンドに付ける見出し。
	headingCommand = "実行するコマンド:"
	// itemIndent は見出しの下に並べる項目の字下げ。
	itemIndent = "  "
	// promptLine は最終行の問い。**既定がキャンセルであることを N の大文字で示す。**
	promptLine = "実行しますか? [y/N]"
)

// ConfirmInput は確認ダイアログの中身。
//
// screens.md に現れる確認（停止 / 強制停止 / 削除 / バージョン更新 / クリーンアップ /
// 追加のプレビュー / 設定の書き込み）はすべてこの 4 ブロックで表せる。ブロックを
// 増やしたくなったら、まず既存のどれかに収まらないかを疑うこと。確認の構造が
// 操作ごとに分かれると、確認を経ない経路が紛れ込んでも気付けなくなる（FR-30）。
//
// **空のブロックは見出しごと落ちる。** 中身の無い「対象:」だけの行を出すと、
// 対象を渡し忘れたのか、そもそも対象を持たない確認なのかを読み分けられない。
type ConfirmInput struct {
	Title   string   // 「停止の確認」
	Targets []string // 対象の一覧
	Impact  []string // 影響・警告（危険色で描く）
	Command []string // 実行するコマンド全文（マスク済み）
	Note    []string // 補足（「復元できません」など）
}

// ConfirmedMsg は確認の決定を page へ通知する。OK が偽ならキャンセルである。
//
// **キャンセルも必ず発行する。** ダイアログ自身は閉じず、閉じる判断は page が
// 行う（page/overlay.go のモーダルの重なり）。決定を握り潰して黙って閉じると、
// 「実行しない」と決めたことが page に届かず、開いた側が後始末（対象の選択解除・
// 状態行の表示戻し）を行う機会を失う。
//
// organism.ChosenMsg に倣って決定は Msg で返し、ダイアログはドメイン層を呼ばない。
// 実行するコマンドを持っていても、実行するのは page の役割である。
type ConfirmedMsg struct {
	OK bool
}

// Confirm は破壊的操作の確認ダイアログ。ローカル状態を持たない（既定はキャンセル）。
//
// 見出し（Title）はモーダルの枠が描くため、View が返すのは中身だけである。
type Confirm struct {
	in     ConfirmInput
	keys   keymap.Confirm
	styles token.Styles
	width  int
	height int
}

// NewConfirm は確認ダイアログを組み立てる。
//
// キー定義は受け取るだけで組み立てない。y / n / enter / esc の割り当てとその理由は
// keymap.NewConfirm が持つ（キーの定義は ui/keymap に集約する）。
func NewConfirm(keys keymap.Confirm, s token.Styles) Confirm {
	return Confirm{
		in:     ConfirmInput{Title: "", Targets: nil, Impact: nil, Command: nil, Note: nil},
		keys:   keys,
		styles: s,
		width:  0,
		height: 0,
	}
}

// SetInput は確認の中身を差し替える。
func (c *Confirm) SetInput(in ConfirmInput) {
	c.in = in
}

// Restyle は配色とキー定義を差し替える。中身は保つ。
//
// 作り直さずに差し替えるのは、共有状態が 3 秒ごとに配られるためである
// （organism.ChoiceList.Restyle と同じ理由）。確認の最中に作り直すと、
// 3 秒ごとに中身が空へ戻る。
func (c *Confirm) Restyle(keys keymap.Confirm, s token.Styles) {
	c.keys, c.styles = keys, s
}

// SetSize はダイアログの中身に配られた領域を設定する。
//
// 高さも受け取るのは、収まらないときに落とす行をこちらで選ぶためである
// （fitHeight。枠に任せると y/N の行から先に消える）。
func (c *Confirm) SetSize(w, h int) {
	c.width, c.height = w, h
}

// Title は見出しを返す。枠（template.Modal）に渡すために公開する。
func (c Confirm) Title() string { return c.in.Title }

// Update は確認のキーを処理する。
//
// esc をここで解釈するため、page はこのダイアログの Modal.HandlesBack に真を
// 返させること。返さないと esc は Overlay がモーダルを 1 枚閉じる操作として消費し、
// ConfirmedMsg{OK: false} が page へ届かない（page/modal.go の HandlesBack）。
func (c Confirm) Update(msg tea.Msg) (Confirm, tea.Cmd) {
	press, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return c, nil
	}

	switch {
	case key.Matches(press, c.keys.Yes):
		return c, confirmed(true)
	case key.Matches(press, c.keys.No):
		return c, confirmed(false)
	}
	return c, nil
}

// View は確認の中身を返す。空のブロックは見出しごと落とし、最後に y/N の行を置く。
func (c Confirm) View() string {
	body := c.blocks()
	if len(body) > 0 {
		// 空行は y/N の行と対にせず本文側の末尾に置く。高さが足りないときに
		// 真っ先に落ちる行がこの空行になり、残す行数（tail）を 1 行に保てる。
		body = append(body, "")
	}
	return strings.Join(fitHeight(body, []string{c.fit(promptLine)}, c.height), "\n")
}

// Hints はフッタに出すキーヒントを返す。
//
// esc と enter は n と同じキャンセルであり、keymap.Confirm.No が 1 つの Binding に
// まとめて持つ（Help().Key は "n"）。同じ結果になるキーを 3 つ並べるとフッタ 1 行目の
// 幅 80 に収まらず、他の画面のヒントを押し出す。
func (c Confirm) Hints() []atom.Hint {
	return []atom.Hint{
		hint(c.keys.Yes, "実行"),
		hint(c.keys.No, "キャンセル"),
	}
}

// blocks は空でないブロックを空行で区切って並べた行を返す。
func (c Confirm) blocks() []string {
	groups := [][]string{
		c.listBlock(headingTargets, c.in.Targets),
		c.styledBlock(c.in.Impact, c.styles.Danger),
		c.listBlock(headingCommand, c.in.Command),
		c.styledBlock(c.in.Note, c.styles.Muted),
	}

	out := make([]string, 0, len(c.in.Targets)+len(c.in.Impact)+len(c.in.Command)+len(c.in.Note)+6)
	for _, g := range groups {
		if len(g) == 0 {
			continue
		}
		if len(out) > 0 {
			out = append(out, "")
		}
		out = append(out, g...)
	}
	return out
}

// listBlock は見出しと字下げした項目の並びを返す。項目が無ければ見出しごと落とす。
//
// 見出しを Header（補足色の太字）で描くのは、対象やコマンドそのものより弱く
// 見せるためである。読むべきはブロックの中身であり、見出しは区分の手がかりに過ぎない。
func (c Confirm) listBlock(heading string, items []string) []string {
	if len(items) == 0 {
		return nil
	}

	lines := make([]string, 0, len(items)+1)
	lines = append(lines, c.styles.Header.Render(c.fit(heading)))
	for _, item := range items {
		lines = append(lines, c.fit(itemIndent+item))
	}
	return lines
}

// styledBlock は各行を同じスタイルで描いたブロックを返す。
func (c Confirm) styledBlock(items []string, style lipgloss.Style) []string {
	if len(items) == 0 {
		return nil
	}

	lines := make([]string, 0, len(items))
	for _, item := range items {
		lines = append(lines, style.Render(c.fit(item)))
	}
	return lines
}

// fit は 1 行を幅に収める。装飾する前に呼ぶこと。
//
// 装飾済みの文字列を切り詰めると中略記号が装飾の外側に付く（atom.Cell の契約）ため、
// 素の文字列で中略してから装飾する。**幅を超える行は出さない。** 超えると枠の中で
// 折り返して行数が増え、モーダルの下辺が領域の外へ押し出される（template.Modal）。
//
// 幅が未設定（0 以下）のときは切り詰めない。atom.Truncate に 0 を渡すと空文字が
// 返り、大きさが配られる前の 1 フレームで中身が丸ごと消える。
func (c Confirm) fit(line string) string {
	if c.width <= 0 {
		return line
	}
	return atom.Truncate(line, c.width)
}

// confirmed は決定を通知する Cmd を返す。
func confirmed(ok bool) tea.Cmd {
	msg := ConfirmedMsg{OK: ok}
	return func() tea.Msg { return msg }
}
