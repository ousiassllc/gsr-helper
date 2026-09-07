package report

import (
	"slices"
	"strings"

	"charm.land/lipgloss/v2"
)

// Wrap は報告の行を width（表示幅）で折り返す。
//
// **切り詰めではなく折り返す。** 報告の行が長くなるのは失敗したときであり、そこが
// いちばん読みたい場所である。切り詰めると失敗の理由と `→` の Hint 行の末尾から
// 順に消えていく（org の tarball 取得が `[HTTP 403]: GET https://api.gith…` で
// 切れ、権限不足の理由も対処コマンドも読めなかった実例がある）。
//
// **width が 1 未満なら何もしない。** 最初のリサイズが届く前は BodyW が 0 であり、
// そこで 1 文字ずつに割ると報告が縦に伸び切る。
//
// 幅はルーン数ではなく lipgloss の表示幅で数える。報告は日本語を含み、全角を 1 と
// 数えると桁が合わない。
func Wrap(lines []string, width int) []string {
	if width < 1 {
		return slices.Clone(lines)
	}

	out := make([]string, 0, len(lines))
	for _, l := range lines {
		out = append(out, wrapOne(l, width)...)
	}
	return out
}

// wrapOne は 1 行を width で折り返す。
//
// 語の境界は見ない。報告に載るのはコマンド行・URL・識別子であり、空白で折ると
// 1 行に収まる幅を使い切れないうえ、空白を含まない長い URL では効果が無い。
func wrapOne(line string, width int) []string {
	if lipgloss.Width(line) <= width {
		return []string{line}
	}

	var out []string
	var b strings.Builder
	for _, r := range line {
		if b.Len() > 0 && lipgloss.Width(b.String()+string(r)) > width {
			out = append(out, b.String())
			b.Reset()
		}
		b.WriteRune(r)
	}
	if b.Len() > 0 {
		out = append(out, b.String())
	}
	return out
}
