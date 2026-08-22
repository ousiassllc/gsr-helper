package template

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// ModalInput はモーダルの内容と、モーダルを置ける領域の大きさ。
//
// Width / Height はモーダル自身の大きさではなく、モーダルを中央に置く領域
// （template.BodySize が返す本体の領域）である。
type ModalInput struct {
	Title  string
	Body   string
	Width  int
	Height int
}

const (
	// modalFrameWidth は枠線と左右余白が使う幅（左右それぞれ 枠 1 + 余白 1）。
	modalFrameWidth = 4
	// modalFrameHeight は枠線が使う行数（上下 1 行ずつ）。上下の余白は付けない。
	modalFrameHeight = 2
	// modalMinInner は枠を描くために最低限必要な内側の幅。
	modalMinInner = 4
	// modalTitleGap は見出しと本文の間に空ける行数。
	modalTitleGap = 1
)

// ModalPadding は Modal の枠と見出しが使う幅と行数を返す。
//
// モーダルの中身へ配る領域を page が算出するために公開する。非公開の定数の写しを
// page 側に持たせず、真実をこのパッケージに 1 つだけ置く（BodySize と同じ形）。
// 幅は左右それぞれの枠線 1 + 余白 1、行数は上下の枠線 2 + 見出し 1 + 空行 1 である。
func ModalPadding() (w, h int) {
	return modalFrameWidth, modalFrameHeight + modalTitleGap + 1
}

// Modal は中央寄せのオーバーレイ枠を返す。
//
// 本体の上に重ねる合成（lipgloss の Canvas / Layer）は行わない。モーダル表示中は
// 本体を描かず、この結果で置き換える方式にする。screens.md のモックはいずれも
// モーダルが画面を占める形で合成が必須ではなく、合成した結果は行単位の期待値で
// 検証しにくいためである。小さなダイアログを本体に重ねる必要が出た場合は、
// この関数の実装だけを差し替えれば済む。
//
// 内容が領域に収まらない場合は切り詰める。枠を描く余地が無い狭い領域では枠を
// 諦め、内容だけを領域に収めて返す（幅不足でも panic しない）。
func Modal(in ModalInput) string {
	if in.Width <= 0 || in.Height <= 0 {
		return ""
	}

	// 中身に使える幅と行数。ModalPadding が page へ返すのと同じ引き算であり、
	// page が配った中身がそのまま収まる大きさである。
	innerWidth := in.Width - modalFrameWidth
	innerHeight := in.Height - modalFrameHeight
	if innerWidth < modalMinInner || innerHeight < 1 {
		return clip(modalContent(in, in.Height, in.Width), in.Width, in.Height)
	}

	// Width() / Height() には領域そのものを渡す。lipgloss のこの 2 つは枠線と余白を
	// 含めた外側の大きさを指定するもので、内側の大きさではない。内側（innerWidth /
	// innerHeight）を渡すと枠が modalFrameWidth / modalFrameHeight の分だけ小さくなり、
	// ModalPadding を元に組まれた中身が折り返す・末尾が切れる。
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		Padding(0, 1).
		Width(in.Width).
		Height(in.Height).
		MaxWidth(in.Width).
		MaxHeight(in.Height).
		Render(modalContent(in, innerHeight, innerWidth))
	return lipgloss.Place(in.Width, in.Height, lipgloss.Center, lipgloss.Center, box)
}

// modalContent は見出しと本文を height 行・width セルに収めて返す。
//
// 行の幅も切るのは、width を超える行が lipgloss の枠の中で折り返し、行数が増えて
// 枠の下辺（╰…╯）が Height / MaxHeight の外へ押し出されるためである。行数だけを
// 数えても、折り返した 1 行が 2 行を占めるので枠が閉じない。切り詰めを lipgloss に
// 任せるのは clip と同じ理由（装飾済みの文字列の ANSI 列を壊さない）である。
func modalContent(in ModalInput, height, width int) string {
	lines := make([]string, 0, height)
	if in.Title != "" {
		lines = append(lines, in.Title)
		if height > modalTitleGap+1 {
			lines = append(lines, "")
		}
	}
	if in.Body != "" {
		lines = append(lines, strings.Split(in.Body, "\n")...)
	}
	if len(lines) > height {
		lines = lines[:height]
	}
	fit := lipgloss.NewStyle().MaxWidth(max(width, 1))
	for i, line := range lines {
		lines[i] = fit.Render(line)
	}
	return strings.Join(lines, "\n")
}

// clip は文字列を指定の幅と高さに収める。
//
// 幅の切り詰めを lipgloss に任せるのは、装飾済みの文字列に含まれる ANSI 列を
// 壊さずに切るためである。
func clip(s string, width, height int) string {
	return lipgloss.NewStyle().MaxWidth(width).MaxHeight(height).Render(s)
}
