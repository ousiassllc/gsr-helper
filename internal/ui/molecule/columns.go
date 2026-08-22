// Package molecule は atom を並べた「意味のある 1 行 / 1 区画」を組み立てる。
//
// bubbletea / bubbles を import せず、ドメインの型も受け取らない。表示に必要な
// 値だけを落とした構造体とプリミティブを受け取ることで、表示部品がドメイン
// モデルの変更に引きずられないようにする。molecule 同士は参照しない。
package molecule

import (
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

// Columns は幅に収まる列だけを選ぶ。ヘッダと行が同じ結果を使うことで桁ずれを防ぐ。
//
// 落とす順は次の 2 段で、収まった時点で止まる。
//
//  1. rules.Drop の順（Runners タブなら _WORK → VERSION → MANAGED → SCOPE）。
//     順は区画が宣言する（token.ColumnRules）。共有の 1 本にすると、タブを足すたびに
//     その並びを直すことになる。
//  2. それでも収まらない場合は残った列の末尾から。右側の列ほど補足的な情報である。
//     宣言された順だけに頼ると、孤児ユニットの NOTE や Jobs の WORKER PID のように
//     「落とせる列が 1 つも無い」区画ができて、幅 60 でも桁が溢れる。
//
// どちらの段でも rules.Keep の列は落とさない。**all が空でなければ結果も
// 空にならない**（最後の 1 列は幅が足りなくても残す）。列が 0 個になると見出しも
// 行も描けず、Jobs タブのように常時表示の列を持たない区画が空表になるためである。
// そのため結果は width を超え得る。超えるのは最小の列でも収まらない幅に限り、
// 表示不能とするかの判断は template.Frame の責務である。
func Columns(all []token.Column, width int, rules token.ColumnRules) []token.Column {
	keep := make([]token.Column, len(all))
	copy(keep, all)

	always := rules.Keep
	for _, id := range rules.Drop {
		// 1 列だけになったらそこで止める（列が 0 個の一覧は描けない）。rules.Keep が
		// 空でも契約が守られるように、末尾落としと同じ歯止めをここにも置く。
		if len(keep) <= 1 || columnsFit(keep, width) {
			return keep
		}
		if hasColumnID(always, id) {
			// 常に表示する列は落とさない。Drop と Keep が食い違っても
			// 常時表示の保証が崩れないようにするための防御である。
			continue
		}
		keep = dropColumn(keep, id)
	}

	// 末尾から落とすのは、右側の列ほど補足的な情報であるためである。1 列だけに
	// なったらそこで止める（列が 0 個の一覧は描けない）。
	for i := len(keep) - 1; i >= 0 && len(keep) > 1 && !columnsFit(keep, width); i-- {
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
