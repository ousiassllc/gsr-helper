// Package molecule は atom を並べた「意味のある 1 行 / 1 区画」を組み立てる。
//
// bubbletea / bubbles を import せず、ドメインの型も受け取らない。表示に必要な
// 値だけを落とした構造体とプリミティブを受け取ることで、表示部品がドメイン
// モデルの変更に引きずられないようにする。molecule 同士は参照しない。
package molecule

import (
	"github.com/ousiassllc/gsr-helper/internal/ui/atom"
	"github.com/ousiassllc/gsr-helper/internal/ui/token"
)

const (
	// columnGutter は列と列の間隔。Columns の幅判定と organism.Table のセル余白で
	// 同じ値を使う。
	columnGutter = 1
	// columnPrefix は行頭のカーソルとチェックボックスに使う幅。
	//
	// 内訳はカーソル 1（token.IconCursor）+ 間隔 1 + チェックボックス 3
	// （token.IconChecked の "[x]"）+ 間隔 1 = 6 セル。選択モードでないときも
	// チェックボックスの分を見積もるのは、選択モードに入った瞬間に右端が
	// 溢れることを防ぐためである。atom から幅を取れないので定数にする。
	columnPrefix = 6
)

// Columns は幅に収まる列だけを選ぶ。
//
// token.ColumnDropOrder の順に落とし、token.ColumnsAlways の列は落とさない。
// ヘッダと行が同じ結果を使うことで桁ずれを防ぐ。
//
// 落とす順に載っていない列（孤児ユニットの NOTE や Jobs の WORKER PID など）は、
// 順を使い切っても収まらない場合に末尾から落とす。落とす順は screens.md が
// Runners タブについて定めたものであり、それだけに頼ると「落とせる列が 1 つも
// 無い」区画ができて、幅 60 でも桁が溢れる。常に表示する列だけになっても
// 収まらない場合はそれを返す（表示不能とするかの判断は template.Frame の責務）。
func Columns(all []token.Column, width int) []token.Column {
	keep := make([]token.Column, len(all))
	copy(keep, all)

	always := token.ColumnsAlways()
	for _, id := range token.ColumnDropOrder() {
		if columnsFit(keep, width) {
			return keep
		}
		if hasColumnID(always, id) {
			// 常に表示する列は落とさない。token 側の 2 つの定義が食い違っても
			// 常時表示の保証が崩れないようにするための防御である。
			continue
		}
		keep = dropColumn(keep, id)
	}

	// 末尾から落とすのは、右側の列ほど補足的な情報であるためである。
	for i := len(keep) - 1; i >= 0 && !columnsFit(keep, width); i-- {
		if hasColumnID(always, keep[i].ID) {
			continue
		}
		keep = dropColumn(keep, keep[i].ID)
	}
	return keep
}

// columnsFit は列が幅に収まるかを返す。
//
// 間隔は列と列の間にだけ入る。最終列の後ろにも数えると 1 セル過大に見積もり、
// 表示を保証する幅（token.WidthTarget）で列が落ちる。
func columnsFit(cols []token.Column, width int) bool {
	if len(cols) == 0 {
		return true
	}

	total := columnPrefix + columnGutter*(len(cols)-1)
	for _, c := range cols {
		total += c.Width
	}
	return total <= width
}

// dropColumn は識別子の一致する列を取り除く。
func dropColumn(cols []token.Column, id string) []token.Column {
	kept := make([]token.Column, 0, len(cols))
	for _, c := range cols {
		if c.ID != id {
			kept = append(kept, c)
		}
	}
	return kept
}

// hasColumnID は識別子が一覧に含まれるかを返す。
func hasColumnID(ids []string, id string) bool {
	for _, v := range ids {
		if v == id {
			return true
		}
	}
	return false
}

// columnAlign は列の寄せ方を返す。
func columnAlign(c token.Column) atom.Align {
	if c.Right {
		return atom.Right
	}
	return atom.Left
}

// styledCell は素の文字列を列幅に揃えてから装飾したセルを返す。
//
// 装飾してから幅を揃えると、切り詰めで ANSI 列が壊れるため atom.Pad しか使えず
// 列幅を超える。bubbles/table は超過分を切り落とすので、ヘッダとの桁ずれや記号の
// 欠落になる。順序を「幅調整 → 装飾」に固定してこれを防ぐ。
func styledCell(text string, role token.RoleToken, c token.Column, s token.Styles) string {
	return s.Style(role).Render(atom.Cell(text, c.Width, columnAlign(c)))
}

// dashCell は値が空のとき「値なし」の記号を薄く描いたセルを返す。
func dashCell(v string, c token.Column, s token.Styles) string {
	if v == "" {
		return styledCell(token.IconNoUnit, token.RoleMuted, c, s)
	}
	return styledCell(v, token.RolePlain, c, s)
}
