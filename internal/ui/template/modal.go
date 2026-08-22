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
	// modalFrameHeight は枠線が使う行数（上下 1 行ずつ）。
	modalFrameHeight = 2
	// modalMinInner は枠を描くために最低限必要な内側の幅。
	modalMinInner = 4
	// modalTitleGap は見出しと本文の間に空ける行数。
	modalTitleGap = 1
)

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

	innerWidth := in.Width - modalFrameWidth
	innerHeight := in.Height - modalFrameHeight
	if innerWidth < modalMinInner || innerHeight < 1 {
		return clip(modalContent(in, in.Height), in.Width, in.Height)
	}

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		Padding(0, 1).
		Width(innerWidth).
		Height(innerHeight).
		MaxWidth(in.Width).
		MaxHeight(in.Height).
		Render(modalContent(in, innerHeight))
	return lipgloss.Place(in.Width, in.Height, lipgloss.Center, lipgloss.Center, box)
}

// modalContent は見出しと本文を height 行に収めて返す。
func modalContent(in ModalInput, height int) string {
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
	return strings.Join(lines, "\n")
}

// clip は文字列を指定の幅と高さに収める。
//
// 幅の切り詰めを lipgloss に任せるのは、装飾済みの文字列に含まれる ANSI 列を
// 壊さずに切るためである。
func clip(s string, width, height int) string {
	return lipgloss.NewStyle().MaxWidth(width).MaxHeight(height).Render(s)
}
