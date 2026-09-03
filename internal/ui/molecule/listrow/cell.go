package listrow

import (
	"github.com/ousiassllc/gsr-helper/internal/ui/atom"
	"github.com/ousiassllc/gsr-helper/internal/ui/token"
)

// 行のセルを 1 つ組み立てる共通部品。行ビルダ 3 つで共有する。

// columnAlign は列の寄せ方を返す。
func columnAlign(c token.Column) atom.Align {
	if c.Right {
		return atom.Right
	}
	return atom.Left
}

// styledCell は素の文字列を列幅に揃えてから装飾したセルを返す。
//
// 順序を「幅調整 → 装飾」に固定するのは、装飾済みの文字列を切り詰めると中略記号が
// 装飾の外側に落ち（atom.Cell の doc）、セルの末尾だけ色が抜けるためである。幅の
// 計算そのものはどちらの順序でも合う（atom は ANSI 列を除いて数える）。
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
