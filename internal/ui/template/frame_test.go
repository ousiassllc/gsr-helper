package template_test

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/ousiassllc/gsr-helper/internal/ui/template"
	"github.com/ousiassllc/gsr-helper/internal/ui/token"
)

func TestBodySize(t *testing.T) {
	tests := []struct {
		name          string
		width, height int
		wantW, wantH  int
	}{
		{"標準の端末", 80, 24, 80, 24 - template.ChromeHeight},
		{"枠と同じ高さ", 80, template.ChromeHeight, 80, 0},
		{"高さが枠に足りない", 80, 3, 80, 0},
		{"高さ 0", 80, 0, 80, 0},
		{"負のサイズ", -5, -5, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w, h := template.BodySize(tt.width, tt.height)
			if w != tt.wantW || h != tt.wantH {
				t.Errorf("BodySize(%d, %d) = (%d, %d), want (%d, %d)",
					tt.width, tt.height, w, h, tt.wantW, tt.wantH)
			}
		})
	}
}

// 本体の行数が指定と違っても、枠が使う行数は ChromeHeight で固定される。
// 本体の行数で全体の高さが動くと、端末の高さを超えて画面が流れる。
func TestFrameKeepsChromeHeight(t *testing.T) {
	const width, height = 80, 24
	_, bodyHeight := template.BodySize(width, height)

	tests := []struct {
		name string
		body string
	}{
		{"本体が空", ""},
		{"本体が 1 行", "一覧"},
		{"本体がちょうど", strings.Repeat("行\n", bodyHeight-1) + "行"},
		{"本体が長すぎる", strings.Repeat("行\n", bodyHeight*2)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := template.Frame(template.FrameInput{
				Header: "gsr-helper  host: build01",
				Tabs:   "[1]Runners [2]Jobs",
				Body:   tt.body,
				Status: "⚠ 警告 2 件",
				Footer: "s:開始 x:停止\nx: root 権限が必要です",
				Width:  width,
				Height: height,
			})
			if got := lipgloss.Height(got); got != height {
				t.Errorf("全体の行数 = %d, want %d", got, height)
			}
		})
	}
}

// 各領域が期待する行位置に出る（screens.md の共通レイアウト）。
//
// 行数の合計だけを見ると、区切り線の本数や領域の並びが変わっても気付けない。
func TestFrameLineOrder(t *testing.T) {
	const width, height = 80, 24
	_, bodyHeight := template.BodySize(width, height)

	got := strings.Split(template.Frame(template.FrameInput{
		Header: "ヘッダ", Tabs: "タブ", Body: "本体", Status: "状態",
		Footer: "フッタ\n理由", Width: width, Height: height,
	}), "\n")
	if len(got) != height {
		t.Fatalf("全体の行数 = %d, want %d", len(got), height)
	}

	div := strings.Repeat(token.IconDivider, width)
	want := map[int]string{
		0: "ヘッダ", 1: "タブ", 2: div, 3: "本体",
		3 + bodyHeight: div,
		height - 3:     "状態",
		height - 2:     "フッタ",
		height - 1:     "理由",
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("%d 行目 = %q, want %q", i, got[i], w)
		}
	}
}

// どの領域の行も幅に収める。幅を超えると端末が折り返し、行数の固定が崩れる。
func TestFrameClipsLinesToWidth(t *testing.T) {
	const width = 80
	long := strings.Repeat("あ", 100)

	got := template.Frame(template.FrameInput{
		Header: long, Tabs: long, Body: long + "\n" + long, Status: long,
		Footer: long + "\n" + long, Width: width, Height: 24,
	})
	for i, line := range strings.Split(got, "\n") {
		if w := lipgloss.Width(line); w > width {
			t.Errorf("%d 行目の表示幅 = %d, want %d 以下", i, w, width)
		}
	}
}

// 高さが枠に足りない場合も行を負にせず、枠の行だけを返す。
func TestFrameWithoutRoomForBody(t *testing.T) {
	got := template.Frame(template.FrameInput{
		Header: "ヘッダ",
		Tabs:   "タブ",
		Body:   "本体",
		Status: "状態",
		Footer: "フッタ\n理由",
		Width:  80,
		Height: 2,
	})
	if got := lipgloss.Height(got); got != template.ChromeHeight {
		t.Errorf("全体の行数 = %d, want %d", got, template.ChromeHeight)
	}
	if strings.Contains(got, "本体") {
		t.Error("本体の領域が無いのに本体が描かれている")
	}
}

// 幅が WidthMin を下回る場合は本体を描かず、表示不能である旨のみを出す。
func TestFrameTooNarrow(t *testing.T) {
	for _, width := range []int{token.WidthMin - 1, 40, 20} {
		got := template.Frame(template.FrameInput{
			Header: "ヘッダ",
			Tabs:   "タブ",
			Body:   "本体の一覧",
			Status: "状態",
			Footer: "フッタ",
			Width:  width,
			Height: 24,
		})
		if strings.Contains(got, "本体の一覧") {
			t.Errorf("幅 %d: 表示不能なのに本体が描かれている", width)
		}
		if strings.Contains(got, "ヘッダ") || strings.Contains(got, "フッタ") {
			t.Errorf("幅 %d: 表示不能なのに枠が描かれている", width)
		}
		if !strings.Contains(got, "表示できません") {
			t.Errorf("幅 %d: 表示不能である旨が出ていない: %q", width, got)
		}
		if lipgloss.Width(got) > width {
			t.Errorf("幅 %d: 表示が領域を超えている（%d）", width, lipgloss.Width(got))
		}
	}
}

// 文字を 1 つも置けない幅でも panic せず、何かを返す。
func TestFrameWithUnusableWidth(t *testing.T) {
	for _, width := range []int{10, 1, 0, -1} {
		got := template.Frame(template.FrameInput{
			Body:   "本体の一覧",
			Width:  width,
			Height: 24,
		})
		if got == "" {
			t.Errorf("幅 %d: 表示不能である旨が空になっている", width)
		}
		if strings.Contains(got, "本体の一覧") {
			t.Errorf("幅 %d: 表示不能なのに本体が描かれている", width)
		}
	}
}

// ちょうど WidthMin では通常の枠を描く（境界値）。
func TestFrameAtWidthMin(t *testing.T) {
	got := template.Frame(template.FrameInput{
		Body:   "本体の一覧",
		Width:  token.WidthMin,
		Height: 24,
	})
	if !strings.Contains(got, "本体の一覧") {
		t.Errorf("幅 %d では本体を描くべきである", token.WidthMin)
	}
}
