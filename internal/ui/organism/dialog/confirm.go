// Package dialog は承認・待機・入力のダイアログを提供する。
//
// organism の部品を性質で分けた 1 つで、破壊的操作の確認（Confirm）・差分の承認
// （DiffApproval）・ドレイン待機（DrainWaiter）・フォーム（Form）を置く。分けたのは
// 1 ディレクトリ 2000 行の上限を分散するためであり、部品同士の依存を増やすためでは
// ない。**organism / organism/table / organism/pane / organism/dialog は、どの向きにも
// import しない。** 必要なものを選んで組み合わせるのは page の役割である。
//
// import するのは atom / molecule / token / keymap と bubbles / bubbletea / lipgloss に
// 限り、page とドメイン層は import しない。型名に階層名を重ねない規約に従い、型は
// Confirm と呼ぶ（ConfirmDialog とはしない）。
package dialog

import (
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/ui/atom"
	"github.com/ousiassllc/gsr-helper/internal/ui/keymap"
	"github.com/ousiassllc/gsr-helper/internal/ui/molecule"
	"github.com/ousiassllc/gsr-helper/internal/ui/token"
)

const (
	// labelTargets は対象の一覧に付ける見出し。
	//
	// 「削除対象」と書かないのは、このダイアログを停止・強制停止・削除・更新・
	// クリーンアップ・設定の書き込みで共有するためである（atomic-design.md の
	// 「Confirm を 1 つに統一する」）。操作ごとの言い換えは Title と Note が持つ。
	labelTargets = "対象:"
	// labelCommand は実行するコマンドに付ける見出し（screens.md の確認ダイアログ）。
	labelCommand = "実行するコマンド:"
	// promptText は末尾の問い。既定がキャンセルであることは [y/N] の大文字が示す。
	promptText = "実行しますか?"
	// indent は見出しにぶら下がる行の字下げ。
	indent = "  "
)

// ConfirmInput は確認ダイアログに出す内容。
//
// screens.md に現れる確認（停止 / 強制停止 / 削除 / バージョン更新 / クリーンアップ /
// 追加のプレビュー / 設定の書き込み）は、すべて「対象・影響・実行するコマンド・y/N」
// という同じ構造を持つ。**個別のダイアログを増やさずこの 1 つに集める**ことで、
// 「確認を経ない破壊的操作の経路を設けない」（FR-30）を構造として守る。
type ConfirmInput struct {
	Title   string   // 見出し（「クリーンアップの確認」）
	Targets []string // 対象の一覧
	Impact  []string // 影響・警告。危険色で描く
	Command []string // 実行するコマンド全文（マスク済み）
	Note    []string // 補足（「削除したファイルは復元できません」など）
}

// DecidedMsg は確認の結果を page へ通知する。
//
// 実行と取消を別の Msg にしないのは、受け取る側が「どちらも来る」ことを型で
// 意識できるようにするためである。取消の Msg を作らないと、page がダイアログを
// 閉じる処理を esc の配送に頼ることになり、n と esc で経路が分かれる。
type DecidedMsg struct {
	Confirmed bool
}

// Confirm は破壊的操作の共通の確認ダイアログ。
//
// ローカル状態は表示する内容・大きさ・配色だけで、カーソルも既定ボタンの強調も
// 持たない。**既定は常にキャンセルであり、状態として保持しない。** 「どちらが
// 選ばれているか」を持つと、開き直したときに前回の選択が残る経路ができる
// （設計原則 5「破壊的操作は影響を表示して y/N（既定 N）」）。
type Confirm struct {
	in     ConfirmInput
	keys   keymap.Set
	styles token.Styles
	width  int
	height int
}

// NewConfirm は確認ダイアログを組み立てる。
//
// keymap.Set をまるごと受け取るのは、キャンセルが Confirm.No だけでなく
// Global.Back（esc）と List.Accept（enter）でも起きるためである。3 つのキーを
// 別々に渡すと、呼び出し側が組み合わせを間違えても気付けない。
func NewConfirm(keys keymap.Set, s token.Styles) Confirm {
	return Confirm{in: ConfirmInput{}, keys: keys, styles: s, width: 0, height: 0}
}

// SetInput は表示する内容を差し替える。
func (c *Confirm) SetInput(in ConfirmInput) {
	c.in = in
}

// Restyle は配色とキー定義を差し替える。内容と大きさは保つ。
//
// 作り直さずに差し替えるのは、背景の明暗が起動後に届き（tea.BackgroundColorMsg）、
// 共有状態が 3 秒ごとに配られるためである。作り直すと確認の途中で内容が消える。
func (c *Confirm) Restyle(keys keymap.Set, s token.Styles) {
	c.keys, c.styles = keys, s
}

// SetSize はダイアログに割り当てられた領域を設定する。
func (c *Confirm) SetSize(w, h int) {
	c.width, c.height = w, h
}

// Title は見出しを返す。枠（template.Modal）が描くため View には含めない。
func (c Confirm) Title() string { return c.in.Title }

// Update は y / n / esc / enter を処理する。
//
// **enter はキャンセルである**（設計原則 5「Enter の連打では進まない」）。一覧や
// 詳細画面から続けて enter を打っている流れのまま破壊的操作へ到達しないよう、
// 実行に割り当てるのは y だけにする。
//
// 上記以外のキーでは何も発行しない。ダイアログを開いている間は背後へキーが
// 流れない（page/overlay.go）ため、ここで握り潰しても操作が失われることはない。
func (c Confirm) Update(msg tea.Msg) (Confirm, tea.Cmd) {
	press, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return c, nil
	}

	switch {
	case key.Matches(press, c.keys.Confirm.Yes):
		return c, decided(true)
	case key.Matches(press, c.keys.Confirm.No, c.keys.Global.Back, c.keys.List.Accept):
		return c, decided(false)
	default:
		return c, nil
	}
}

// decided は決定を通知する Cmd を返す。
func decided(confirmed bool) tea.Cmd {
	msg := DecidedMsg{Confirmed: confirmed}
	return func() tea.Msg { return msg }
}

// View は対象・影響・コマンド・補足・問いを縦に並べて返す。
//
// 並びは screens.md の確認ダイアログのとおりで、影響を対象の直後に置く。何が
// 起きるのかを読む前にコマンドの詳細が挟まると、判断に必要な情報が下へ流れる。
func (c Confirm) View() string {
	lines := make([]string, 0, len(c.in.Targets)+len(c.in.Impact)+len(c.in.Command)+len(c.in.Note)+8)

	lines = c.appendSection(lines, labelTargets, c.in.Targets, token.RolePlain)
	lines = c.appendSection(lines, "", c.in.Impact, token.RoleDanger)
	if block := molecule.CommandBlock(c.in.Command, c.width, c.styles); block != "" {
		lines = append(lines, blankBefore(lines)...)
		lines = append(lines, c.fit(labelCommand))
		lines = append(lines, strings.Split(block, "\n")...)
	}
	lines = c.appendSection(lines, "", c.in.Note, token.RolePlain)

	lines = append(lines, blankBefore(lines)...)
	lines = append(lines, c.fit(c.prompt()))

	return strings.Join(c.clip(lines), "\n")
}

// appendSection は見出しと本文を積む。本文が無ければ何も積まない。
//
// 見出しが空の区画（影響・補足）は字下げせず左端から描く。対象とコマンドだけを
// 字下げするのは、この 2 つが見出しにぶら下がる一覧だからである。
func (c Confirm) appendSection(lines []string, label string, body []string, role token.RoleToken) []string {
	if len(body) == 0 {
		return lines
	}

	lines = append(lines, blankBefore(lines)...)
	prefix := ""
	if label != "" {
		lines = append(lines, c.fit(label))
		prefix = indent
	}
	for _, b := range body {
		// 幅に収めてから装飾する。装飾済みの文字列を切り詰めると ANSI 列が壊れる
		// （atom.Truncate の契約）。
		lines = append(lines, c.styles.Style(role).Render(c.fit(prefix+b)))
	}
	return lines
}

// blankBefore は区画の前に入れる空行を返す。先頭の区画には入れない。
func blankBefore(lines []string) []string {
	if len(lines) == 0 {
		return nil
	}
	return []string{""}
}

// prompt は末尾の問いを返す。キーの表記は keymap の定義から引く。
//
// キャンセル側を大文字で書くのは、既定がキャンセルであることを色に頼らず示すため
// である（設計原則 4 / 5）。文字を書き換えるだけなので、押すキーは小文字のままである。
func (c Confirm) prompt() string {
	yes := bindingKey(c.keys.Confirm.Yes)
	no := strings.ToUpper(bindingKey(c.keys.Confirm.No))
	return promptText + " [" + yes + "/" + no + "]"
}

// fit は 1 行を幅に収める。幅が未設定（0 以下）なら何もしない。
func (c Confirm) fit(s string) string {
	if c.width <= 0 {
		return s
	}
	return atom.Truncate(s, c.width)
}

// clip は高さに収まらない行を落とし、続きがあることを中略記号で示す。
//
// ダイアログはスクロールしない。黙って枠に切り落とさせると、実行するコマンドが
// 画面外にあることに気付けないまま y を押す状況を作る。
func (c Confirm) clip(lines []string) []string {
	if c.height <= 0 || len(lines) <= c.height {
		return lines
	}

	kept := make([]string, 0, c.height)
	kept = append(kept, lines[:c.height-1]...)
	return append(kept, c.styles.Muted.Render(token.IconEllipsis))
}

// Hints はフッタに出すキーヒントを返す（screens.md の確認ダイアログ）。
//
// 説明文は keymap の定義から引く。ここで書き直すと、キーを差し替えたときに
// フッタだけが古い表記のまま残る。esc / enter も同じキャンセルだが、フッタには
// 出さない。**キャンセルの入口が 3 つ並ぶより、実行が y だけであることが読み取れる
// 方が重要**だからである（幅を使い切らない。設計原則 1）。
func (c Confirm) Hints() []atom.Hint {
	yes, no := c.keys.Confirm.Yes, c.keys.Confirm.No
	return []atom.Hint{
		{Key: bindingKey(yes), Desc: yes.Help().Desc, Enabled: true, Reason: ""},
		{Key: bindingKey(no), Desc: no.Help().Desc, Enabled: true, Reason: ""},
	}
}

// bindingKey は Binding が受け付ける実際のキー文字列を返す。
//
// page.BindingKey と同じ内容だが、organism は page を import しないため
// ここに置く（依存の向きを保つための小さな重複である）。
func bindingKey(b key.Binding) string {
	if ks := b.Keys(); len(ks) > 0 {
		return ks[0]
	}
	return b.Help().Key
}
