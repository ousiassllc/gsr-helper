package dialog

import (
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/ui/atom"
	"github.com/ousiassllc/gsr-helper/internal/ui/keymap"
	"github.com/ousiassllc/gsr-helper/internal/ui/molecule/listrow"
	"github.com/ousiassllc/gsr-helper/internal/ui/token"
)

const (
	// titleDiff は差分承認の見出し（screens.md の Config タブ「変更内容の確認」）。
	titleDiff = "変更内容の確認"
	// titleGap は見出しと対象ファイルのパスの間隔。モックの空白 3 つに合わせる。
	titleGap = "   "
	// labelBackup は退避先に付ける見出し。
	labelBackup = "バックアップ:"
	// promptWrite は末尾の問い。既定がキャンセルであることは [y/N] の大文字が示す。
	promptWrite = "この変更を書き込みますか?"
)

// DiffApprovalInput は差分承認ダイアログに出す内容。
//
// 差分を接頭辞付きの行の並びで受け取るのは、**書き込む内容と画面に出る内容を
// 同じ 1 つの計算結果から出すため**である。UI 側で差分を取り直すと、承認した
// 差分と実際に書き込まれる内容が食い違う経路ができる（molecule/listrow.DiffLine）。
type DiffApprovalInput struct {
	// Path は書き込む対象ファイルのパス（"/opt/runners/build01-1/.env"）。
	Path string
	// Diff は接頭辞付きの差分行（"  " 変更なし / "- " 削除 / "+ " 追加）。
	Diff []string
	// Backup は退避先のパス。空なら退避しないものとして行を出さない。
	Backup string
}

// DiffApproval は設定の書き込みを差分で承認するダイアログ（[FR-35]）。
//
// ローカル状態は表示する内容・大きさ・配色だけで、Confirm と同じく**既定は常に
// キャンセルであり、状態として保持しない**（atomic-design.md の organism 一覧の
// 「ローカル状態: なし（既定はキャンセル）」）。「どちらが選ばれているか」を持つと、
// 開き直したときに前回の選択が残る経路ができる（設計原則 5）。
//
// **スクロールも持たない。** 状態を持たない部品なのでスクロール位置を置く場所が
// 無く、収まらない差分は末尾を落として中略記号（…）で続きがあることを示す。
// 差分が画面に収まらないほど大きい場合に承認だけで済ませてよいかは page の判断で
// あり、必要なら pane.Detail を重ねる（この部品に状態を足さない）。
//
// Confirm と別の型にしているのは、確認ダイアログを増やしたいからではなく、
// 出す情報が「対象・影響・コマンド」ではなく「差分・退避先」だからである。
// 決定の Msg は Confirm と同じ DecidedMsg を使い、受け取る page 側の分岐を増やさない。
type DiffApproval struct {
	in     DiffApprovalInput
	keys   keymap.Set
	styles token.Styles
	width  int
	height int
}

// NewDiffApproval は差分承認ダイアログを組み立てる。
//
// keymap.Set をまるごと受け取る理由は NewConfirm と同じで、キャンセルが
// Confirm.No だけでなく Global.Back（esc）と List.Accept（enter）でも起きるためである。
func NewDiffApproval(keys keymap.Set, s token.Styles) DiffApproval {
	return DiffApproval{in: DiffApprovalInput{Path: "", Diff: nil, Backup: ""}, keys: keys, styles: s, width: 0, height: 0}
}

// SetInput は表示する内容を差し替える。
func (d *DiffApproval) SetInput(in DiffApprovalInput) {
	d.in = in
}

// Restyle は配色とキー定義を差し替える。内容と大きさは保つ。
//
// 作り直さずに差し替えるのは Confirm と同じ理由で、背景の明暗が起動後に届き
// （tea.BackgroundColorMsg）、共有状態が 3 秒ごとに配られるためである。
func (d *DiffApproval) Restyle(keys keymap.Set, s token.Styles) {
	d.keys, d.styles = keys, s
}

// SetSize はダイアログに割り当てられた領域を設定する。
func (d *DiffApproval) SetSize(w, h int) {
	d.width, d.height = w, h
}

// Title は見出しを返す。枠（template.Modal）が描くため View には含めない。
//
// 対象ファイルのパスを見出しに並べるのは screens.md のモックのとおりである。
// **どのファイルへ書くのかは差分そのものと同じくらい重要**であり、本文の先頭に
// 置くと差分が伸びたときに中略の対象になる（本文は末尾から落ちる）。
func (d DiffApproval) Title() string {
	if d.in.Path == "" {
		return titleDiff
	}
	return titleDiff + titleGap + d.in.Path
}

// Update は y / n / esc / enter を処理する。意味は Confirm と揃える。
//
// **enter は書き込みではなくキャンセルである**（設計原則 5「Enter の連打では
// 進まない」）。フォームの確定から続けて enter を打っている流れのまま書き込みへ
// 到達しないよう、書き込みに割り当てるのは y だけにする。
//
// 上記以外のキーでは何も発行しない。ダイアログを開いている間は背後へキーが
// 流れない（page/overlay.go）ため、握り潰しても操作は失われない。
func (d DiffApproval) Update(msg tea.Msg) (DiffApproval, tea.Cmd) {
	press, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return d, nil
	}

	switch {
	case key.Matches(press, d.keys.Confirm.Yes):
		return d, decided(true)
	case key.Matches(press, d.keys.Confirm.No, d.keys.Global.Back, d.keys.List.Accept):
		return d, decided(false)
	default:
		return d, nil
	}
}

// View は差分・退避先・問いを縦に並べて返す。
//
// 退避先と問いは fitHeight に tail として渡す。高さが足りないときに落とすのは
// 差分の末尾だけであり、**「どこへ退避したか」と「書き込むか」は必ず残す**
// （doc.go の fitHeight）。
func (d DiffApproval) View() string {
	tail := d.tail()
	lines := fitHeight(d.diffLines(len(tail)), tail, d.height)
	if d.height > 0 && len(lines) > d.height {
		// 退避先と問いだけで領域を超える極端に狭い高さでは、末尾から残す。
		// 幅・高さの不足で行があふれると枠の下辺が領域の外へ出る（template.Modal）。
		lines = lines[len(lines)-d.height:]
	}
	return strings.Join(lines, "\n")
}

// diffLines は差分を描き、高さに収まらない分を落として中略記号を置く。
//
// 黙って枠に切り落とさせないのは、差分の続きが画面外にあることに気付けないまま
// y を押す状況を作らないためである（Confirm.clip と同じ判断）。
func (d DiffApproval) diffLines(tailHeight int) []string {
	lines := make([]string, 0, len(d.in.Diff)+1)
	for _, raw := range d.in.Diff {
		lines = append(lines, listrow.DiffLine(raw, d.width, d.styles))
	}

	limit := d.height - tailHeight
	if d.height <= 0 || len(lines) <= limit {
		return lines
	}
	if limit <= 0 {
		return nil
	}
	return append(lines[:limit-1:limit-1], d.styles.Muted.Render(token.IconEllipsis))
}

// tail は差分の後ろに置く行（退避先と問い）を返す。
func (d DiffApproval) tail() []string {
	lines := make([]string, 0, 4)
	if d.in.Backup != "" {
		lines = append(lines, "", d.fit(labelBackup+" "+d.in.Backup))
	}
	return append(lines, "", d.fit(d.prompt()))
}

// prompt は末尾の問いを返す。キーの表記は keymap の定義から引く。
//
// キャンセル側を大文字で書くのは、既定がキャンセルであることを色に頼らず示す
// ためである（設計原則 4 / 5）。押すキーは小文字のままである。
func (d DiffApproval) prompt() string {
	yes := bindingKey(d.keys.Confirm.Yes)
	no := strings.ToUpper(bindingKey(d.keys.Confirm.No))
	return promptWrite + " [" + yes + "/" + no + "]"
}

// fit は 1 行を幅に収める。幅が未設定（0 以下）なら何もしない。
func (d DiffApproval) fit(s string) string {
	if d.width <= 0 {
		return s
	}
	return atom.Truncate(s, d.width)
}

// Hints はフッタに出すキーヒントを返す。
//
// 説明文は keymap の定義から引く。esc / enter も同じキャンセルだが出さない。
// **キャンセルの入口が 3 つ並ぶより、書き込みが y だけであることが読み取れる方が
// 重要**だからである（Confirm.Hints と揃える）。
func (d DiffApproval) Hints() []atom.Hint {
	yes, no := d.keys.Confirm.Yes, d.keys.Confirm.No
	return []atom.Hint{hint(yes, yes.Help().Desc), hint(no, no.Help().Desc)}
}
