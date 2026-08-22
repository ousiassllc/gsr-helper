// Package atom は画面の最小の表示単位を組み立てる純粋関数を提供する。
//
// bubbletea / bubbles を import しないため、戻り値を期待文字列と比較するだけで
// テストできる。**文字列の表示幅を数えるのはこのパッケージだけ**であり、
// 上位の階層は「どの列を何文字幅で置くか」を決めるだけで幅を数えない。
package atom

import (
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/ousiassllc/gsr-helper/internal/ui/token"
)

// Align はセル内の寄せ方。
type Align int

// Align の取り得る値。
const (
	Left Align = iota
	Right
)

// Cell は文字列を指定幅に揃えた 1 セルを返す。幅を超える分は Truncate が中略する。
//
// 装飾済み（ANSI 列を含む）の文字列を渡してもよい。中略は lipgloss に委ねており、
// ANSI 列も書記素も割らない（Truncate の doc と TestTruncateKeepsANSISequenceIntact）。
// ただし**中略記号は装飾の外側に付く**（`ESC[..m● ac ESC[m…`）ので、中略される幅で
// 装飾を末尾まで届かせたい場合は装飾前の文字列を渡して呼び出し側で装飾する。
//
// 幅を超えても中略させたくない場合は Pad を使う（Pad は切り詰めない）。
func Cell(s string, width int, a Align) string {
	if width <= 0 {
		return ""
	}
	return Pad(Truncate(s, width), width, a)
}

// Pad は文字列を指定幅まで空白で埋める。幅を超える場合は切り詰めずそのまま返す。
// 切り詰めないため、装飾済みの文字列にも使える。
func Pad(s string, width int, a Align) string {
	gap := width - lipgloss.Width(s)
	if gap <= 0 {
		return s
	}
	if a == Right {
		return strings.Repeat(" ", gap) + s
	}
	return s + strings.Repeat(" ", gap)
}

// Truncate は文字列を指定幅に収める。収まらない場合は末尾を中略記号に置き換える。
//
// 全角文字の境界で 1 セル余ることがあるが、埋めるのは Pad の役割である。
//
// 切る位置の判断を lipgloss（MaxWidth）に任せるのは、装飾済みの文字列に含まれる
// ANSI 列を途中で割らないためである。自前に rune を数えると CSI の途中で切れて
// 端末が後続の出力を飲み込む。molecule.KeyBar は装飾済みのキーヒントを Join へ
// 渡し、Join は幅が足りない分をここで中略するので、この経路は実際に起きる。
// あわせて書記素（結合文字・ZWJ 絵文字）も分割されなくなる。
func Truncate(s string, width int) string {
	if width <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= width {
		return s
	}
	limit := width - lipgloss.Width(token.IconEllipsis)
	if limit <= 0 {
		return token.IconEllipsis
	}
	return lipgloss.NewStyle().MaxWidth(limit).Render(s) + token.IconEllipsis
}

// Justify は左寄せと右寄せの文字列を指定幅の 1 行に収める。
//
// 幅が足りない場合も右側を落とさず空白 1 つで詰める。右側には操作できない
// 理由が入るため、幅の都合で理由を消さない（screens.md の無効な操作の表示）。
//
// **そのため戻り値は width を超え得る**（left + " " + right が width より長い場合。
// molecule.ActionRow を狭い幅で使う経路で起きる）。理由の文字列を消さないことを
// 優先した結果である。幅に収める責任は行を組む側にあり、molecule.ActionRow は
// 理由の末尾を Truncate で中略する。template.Frame の切り詰めは最後の防波堤として残る。
func Justify(left, right string, width int) string {
	if right == "" {
		return left
	}
	gap := width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}
	return left + strings.Repeat(" ", gap) + right
}

// Join は parts を sep でつなぎ、width に収まらない分を落として overflow を付ける。
//
// フッタのキーヒントのように「収まらない分は ?:ヘルプ に集約する」表示に使う。
// overflow が空の場合は収まらない分をそのまま落とす。
//
// parts 全体が width に収まる場合は overflow の分を予算から引かない。無条件に
// 先取りすると、ちょうど収まる幅でも全ての parts が落ちて overflow だけが残る。
func Join(parts []string, sep string, width int, overflow string) string {
	if width <= 0 {
		return ""
	}
	if full := strings.Join(parts, sep); lipgloss.Width(full) <= width {
		return full
	}

	budget := width
	if overflow != "" {
		budget -= lipgloss.Width(sep) + lipgloss.Width(overflow)
	}
	if budget < 0 {
		// overflow だけでも収まらない幅。表示不能の判断は template.Frame に任せ、
		// ここでは中略した overflow を返す。
		return Truncate(overflow, width)
	}

	var b strings.Builder
	used := 0
	dropped := false
	for _, p := range parts {
		need := lipgloss.Width(p)
		if used > 0 {
			need += lipgloss.Width(sep)
		}
		if used+need > budget {
			dropped = true
			break
		}
		if used > 0 {
			b.WriteString(sep)
		}
		b.WriteString(p)
		used += need
	}
	if dropped && overflow != "" {
		if used > 0 {
			b.WriteString(sep)
		}
		b.WriteString(overflow)
	}
	return b.String()
}

// Path はパスを最大幅に収める。中間を中略し、末尾（重要な部分）を残す。
func Path(p string, width int) string {
	if width <= 0 || p == "" {
		return ""
	}
	if lipgloss.Width(p) <= width {
		return p
	}

	segs := strings.Split(p, "/")
	// 先頭要素（絶対パスなら空文字）と末尾側の要素を残し、間を中略する。
	head := segs[0]
	if head == "" && len(segs) > 1 {
		head = "/" + segs[1]
		segs = segs[1:]
	}
	middle := "/" + token.IconEllipsis
	best := ""
	for i := len(segs) - 1; i >= 1; i-- {
		cand := head + middle + "/" + strings.Join(segs[i:], "/")
		if lipgloss.Width(cand) > width {
			break
		}
		best = cand
	}
	if best != "" {
		return best
	}
	// 先頭を残す余裕が無い場合は末尾だけを残す。
	return token.IconEllipsis + tail(p, width-lipgloss.Width(token.IconEllipsis))
}

// tail は文字列の末尾から width セル分を返す。
//
// 末尾を数えるのではなく、**先頭を lipgloss で落とした残り**を返す。lipgloss には
// 末尾から切る手立てが無いが、先頭からの切り詰め（MaxWidth）は書記素を割らないので、
// 落とす幅を 1 セルずつ広げて残りが収まった時点で止めれば末尾側も書記素の境界に
// そろう。rune を末尾から数えると、結合文字や ZWJ 絵文字が途中で分割される
// （Truncate と同じ理由。_work のパスに絵文字が混じると起きる）。
//
// 落とす幅を広げる回数は文字列の表示幅で頭打ちになる。パスの表示は 1 行ぶんなので
// 実際の反復は数回で終わる。
func tail(s string, width int) string {
	if width <= 0 {
		return ""
	}
	total := lipgloss.Width(s)
	if total <= width {
		return s
	}

	for drop := total - width; drop <= total; drop++ {
		head := lipgloss.NewStyle().MaxWidth(drop).Render(s)
		rest, ok := strings.CutPrefix(s, head)
		if !ok {
			// 切り詰めが元の文字列の接頭辞にならない形（想定外）。末尾を返さない。
			return ""
		}
		if lipgloss.Width(rest) <= width {
			return rest
		}
	}
	return ""
}

// Divider は区画の区切り線を返す。title が空なら線だけを返す。
func Divider(width int, title string, s token.Styles) string {
	if width <= 0 {
		return ""
	}
	if title == "" {
		return s.Divider.Render(strings.Repeat(token.IconDivider, width))
	}

	head := token.IconDivider + " " + title + " "
	rest := width - lipgloss.Width(head)
	if rest < 0 {
		return s.Divider.Render(Truncate(head, width))
	}
	return s.Divider.Render(head + strings.Repeat(token.IconDivider, rest))
}
