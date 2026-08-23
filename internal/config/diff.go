package config

import "strings"

// 差分の行頭に付ける印。organism/dialog.DiffApproval と molecule/listrow.DiffLine は
// この 3 つで行の種類を見分ける（描画側は再度の突き合わせをしない）。
const (
	// MarkContext は変更のない行の印。
	MarkContext = "  "
	// MarkRemove は取り除かれる行の印。
	MarkRemove = "- "
	// MarkAdd は加えられる行の印。
	MarkAdd = "+ "
)

// maxDiffLines は行ごとの突き合わせを行う上限。
//
// 突き合わせは行数の積に比例する表を使うため、上限が無いと壊れた巨大ファイルを
// 開いただけで固まる。.env / .path / drop-in はいずれも数十行のファイルであり、
// これを超える入力は差分を読む場面ではない。超えた場合は全行の入れ替えとして出す。
const maxDiffLines = 1000

// Diff は before から after への差分を、書き込み前のプレビュー用に組み立てる。
//
// 返す文字列は 1 行につき MarkContext / MarkRemove / MarkAdd のいずれかで始まる
// （画面仕様の「変更内容の確認」のモック）。表示の体裁（色・折り返し・切り詰め）は
// 持たない。ドメイン層が色を持つと、同じ差分を別の場所へ出すたびに体裁が分岐する。
//
// 末尾の改行の有無は行数に影響させない。"a\n" と "a" はどちらも 1 行として扱う。
// 改行だけの違いを差分として見せても、利用者に取れる行動が無いためである。
func Diff(before, after string) string {
	old, cur := splitLines(before), splitLines(after)

	var b strings.Builder
	for _, l := range diffLines(old, cur) {
		b.WriteString(l)
		b.WriteString("\n")
	}
	return b.String()
}

// splitLines は改行で分け、末尾の改行が生む空要素だけを落とす。
func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	s = strings.TrimSuffix(s, "\n")
	out := strings.Split(s, "\n")
	for i, l := range out {
		out[i] = strings.TrimSuffix(l, "\r")
	}
	return out
}

// diffLines は 2 つの行列から印付きの行を組み立てる。
func diffLines(old, cur []string) []string {
	if len(old) > maxDiffLines || len(cur) > maxDiffLines {
		return replaceAll(old, cur)
	}

	table := lcs(old, cur)
	out := make([]string, 0, len(old)+len(cur))
	i, j := 0, 0

	for i < len(old) && j < len(cur) {
		switch {
		case old[i] == cur[j]:
			out = append(out, MarkContext+old[i])
			i++
			j++
		case table[i+1][j] >= table[i][j+1]:
			out = append(out, MarkRemove+old[i])
			i++
		default:
			out = append(out, MarkAdd+cur[j])
			j++
		}
	}
	for ; i < len(old); i++ {
		out = append(out, MarkRemove+old[i])
	}
	for ; j < len(cur); j++ {
		out = append(out, MarkAdd+cur[j])
	}
	return out
}

// replaceAll は突き合わせを諦めて全行の入れ替えとして出す。
func replaceAll(old, cur []string) []string {
	out := make([]string, 0, len(old)+len(cur))
	for _, l := range old {
		out = append(out, MarkRemove+l)
	}
	for _, l := range cur {
		out = append(out, MarkAdd+l)
	}
	return out
}

// lcs は最長共通部分列の長さの表を返す。table[i][j] は old[i:] と cur[j:] の分。
//
// 後ろから埋めるのは、diffLines が前から辿りながら「残りの共通部分が長い方」を
// 選べるようにするためである。
func lcs(old, cur []string) [][]int {
	table := make([][]int, len(old)+1)
	for i := range table {
		table[i] = make([]int, len(cur)+1)
	}

	for i := len(old) - 1; i >= 0; i-- {
		for j := len(cur) - 1; j >= 0; j-- {
			if old[i] == cur[j] {
				table[i][j] = table[i+1][j+1] + 1
				continue
			}
			table[i][j] = max(table[i+1][j], table[i][j+1])
		}
	}
	return table
}
