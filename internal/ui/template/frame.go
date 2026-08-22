// Package template は画面共通の枠を組み立てる。
//
// 枠は中身を知らず、領域の配分だけを行う。organism / page を import しないのは、
// 枠が中身を知ると画面ごとに枠が分岐するためである（atomic-design.md の依存の規則）。
// bubbletea / bubbles も import せず純粋関数に保つことで、幅と高さから算出される
// 領域を期待値との比較だけで検証できる状態を保つ。
package template

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/ousiassllc/gsr-helper/internal/ui/token"
)

// FrameInput は共通レイアウト（screens.md の共通レイアウト）の各領域に流し込む文字列。
//
// どの領域も描画済みの文字列を受け取る。Frame は中身を解釈せず、行数だけを揃える。
type FrameInput struct {
	Header string // molecule.CapsBar の結果
	Tabs   string // molecule.TabBar の結果
	Body   string // page の描画結果
	Status string // 孤児ユニット件数 / 警告件数 / 選択件数 / 入力中
	Footer string // molecule.KeyBar の結果（2 行）
	Width  int
	Height int
}

// ChromeHeight は本体以外が使う行数（ヘッダ 1 + タブ 1 + 区切り 1 + 区切り 1 +
// 状態行 1 + フッタ 2）。区切り線はタブの下と状態行の上の 2 本である
// （screens.md の共通レイアウト）。
//
// 固定値にするのは、フッタが 1 行のときと 2 行のときで本体の高さが動くと
// bubbles/table の行数がフレームごとに変わり、表示が上下に跳ねるためである。
// molecule.KeyBar は理由が無いときも空行を返して常に 2 行になる。
const ChromeHeight = 7

// footerHeight は ChromeHeight のうちフッタが占める行数。
const footerHeight = 2

// Frame は 1 画面分のレイアウトを返す。
//
// 幅が token.WidthMin を下回る場合は本体を描かず、表示不能である旨のみを出す
// （screens.md の端末幅による列の省略）。縮退の判断をここに置くのは、
// 幅が足りない画面ごとに別の縮退表示を作らないためである。
func Frame(in FrameInput) string {
	if in.Width < token.WidthMin {
		return tooNarrow(in.Width)
	}

	_, bodyHeight := BodySize(in.Width, in.Height)

	lines := make([]string, 0, ChromeHeight+bodyHeight)
	lines = append(lines, fit(in.Header, 1, in.Width)...)
	lines = append(lines, fit(in.Tabs, 1, in.Width)...)
	lines = append(lines, divider(in.Width))
	lines = append(lines, fit(in.Body, bodyHeight, in.Width)...)
	lines = append(lines, divider(in.Width))
	lines = append(lines, fit(in.Status, 1, in.Width)...)
	lines = append(lines, fit(in.Footer, footerHeight, in.Width)...)
	return strings.Join(lines, "\n")
}

// BodySize は Body に割り当てられる領域を返す。親 Model が算出して page に渡す。
//
// 高さが足りない場合も負の値を返さない。サイズの真実を 1 箇所に集めるため、
// organism は自分でサイズを問い合わせず、この結果を配られるだけにする。
func BodySize(width, height int) (w, h int) {
	return max(width, 0), max(height-ChromeHeight, 0)
}

// fit は文字列を n 行に揃え、各行を width に収める。足りない行は空行で埋め、
// 超える行は落とす。
//
// 行数を揃えるのは、どの領域が何行を使ったかで全体の行数が変わると、
// 端末の高さを超えて画面が流れてしまうためである。幅も切るのは、行が幅を超えると
// 端末が折り返して 1 行が 2 行を占め、行数を固定した意味が失われるためである。
// 切り詰めを lipgloss に任せるのは、装飾済みの文字列の ANSI 列を壊さないためである。
func fit(s string, n, width int) []string {
	if n <= 0 {
		return nil
	}

	lines := make([]string, n)
	if s != "" {
		copy(lines, strings.Split(s, "\n"))
	}
	clip := lipgloss.NewStyle().MaxWidth(max(width, 1))
	for i, line := range lines {
		lines[i] = clip.Render(line)
	}
	return lines
}

// divider は領域の区切り線を返す。タブの下と状態行の上で同じ線を使う。
//
// 装飾を付けないのは FrameInput にスタイルを持たせないためである。枠が配色を
// 決めると、色の解決を親 Model から配る経路が 2 つになる。
func divider(width int) string {
	return strings.Repeat(token.IconDivider, width)
}

// tooNarrow は幅不足の表示を返す。
//
// 切り詰めずに折り返すのは、幅が足りない理由そのものを落とさないためである。
// 中略した「表示できま…」では、利用者が幅を広げれば直ると判断できない。
func tooNarrow(width int) string {
	msg := fmt.Sprintf("表示できません（幅 %d 以上が必要です。現在 %d）", token.WidthMin, width)
	if width <= 0 {
		return msg
	}
	return lipgloss.NewStyle().Width(width).Render(msg)
}
