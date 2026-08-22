package template_test

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/ousiassllc/gsr-helper/internal/ui/template"
)

// モーダルは与えられた領域と同じ大きさの文字列を返す。本体を描かず置き換える
// 方式のため、領域と大きさが違うと枠の行数が本体の行数と合わなくなる。
func TestModalFillsGivenArea(t *testing.T) {
	tests := []struct {
		name          string
		title         string
		width, height int
	}{
		{"標準の本体領域", "クリーンアップの確認", 80, 18},
		{"横長", "クリーンアップの確認", 120, 10},
		{"最小に近い", "確認", 20, 6},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := template.Modal(template.ModalInput{
				Title:  tt.title,
				Body:   "削除対象:\n  /opt/runners/build01-1/_work/_temp",
				Width:  tt.width,
				Height: tt.height,
			})
			if w := lipgloss.Width(got); w != tt.width {
				t.Errorf("幅 = %d, want %d", w, tt.width)
			}
			if h := lipgloss.Height(got); h != tt.height {
				t.Errorf("高さ = %d, want %d", h, tt.height)
			}
			if !strings.Contains(got, tt.title) {
				t.Errorf("見出し %q が描かれていない", tt.title)
			}
		})
	}
}

// 本体が領域より長い場合は切り詰め、領域を超えない。
func TestModalClipsOverflowingBody(t *testing.T) {
	const width, height = 40, 8
	body := make([]string, 0, 50)
	for i := range 50 {
		body = append(body, strings.Repeat("あ", 60-i%3))
	}
	got := template.Modal(template.ModalInput{
		Title:  "長い本体",
		Body:   strings.Join(body, "\n"),
		Width:  width,
		Height: height,
	})
	if w := lipgloss.Width(got); w > width {
		t.Errorf("幅 = %d, want <= %d", w, width)
	}
	if h := lipgloss.Height(got); h != height {
		t.Errorf("高さ = %d, want %d", h, height)
	}
}

// 枠を描く余地が無い領域でも panic せず、内容を領域に収めて返す。
func TestModalWithTooSmallArea(t *testing.T) {
	tests := []struct {
		name          string
		width, height int
	}{
		{"幅が枠に足りない", 4, 6},
		{"高さが枠に足りない", 40, 1},
		{"1 セル", 1, 1},
		{"高さ 0", 40, 0},
		{"負の領域", -3, -3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := template.Modal(template.ModalInput{
				Title:  "確認",
				Body:   "削除しますか?",
				Width:  tt.width,
				Height: tt.height,
			})
			if tt.width > 0 && lipgloss.Width(got) > tt.width {
				t.Errorf("幅 = %d, want <= %d", lipgloss.Width(got), tt.width)
			}
			if tt.height > 0 && lipgloss.Height(got) > tt.height {
				t.Errorf("高さ = %d, want <= %d", lipgloss.Height(got), tt.height)
			}
			if tt.height <= 0 && got != "" {
				t.Errorf("領域が無いのに描画している: %q", got)
			}
		})
	}
}
