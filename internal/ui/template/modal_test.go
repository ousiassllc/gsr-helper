package template_test

import (
	"strconv"
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
	// 高さだけを見ると足りない。内側幅を超える行が枠の中で折り返すと行数が増え、
	// 下辺が領域から押し出されて ╰…╯ が切り落とされる（高さは MaxHeight で
	// 保たれるので気付けない）。
	lines := strings.Split(got, "\n")
	if !strings.HasPrefix(lines[0], "╭") || !strings.HasPrefix(lines[len(lines)-1], "╰") {
		t.Errorf("枠の上辺・下辺が揃っていない:\n%s", got)
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

// ModalPadding は Modal の枠と見出しが実際に使う幅・行数と一致する。
//
// page はこの値で中身へ配る領域を算出する。写しを持たせず真実を 1 箇所に置くため、
// 返り値が Modal の実装と一致していることを固定する。
func TestModalPaddingMatchesFrame(t *testing.T) {
	const width, height = 60, 12
	padW, padH := template.ModalPadding()
	if padW <= 0 || padH <= 0 {
		t.Fatalf("ModalPadding() = (%d, %d), want 正の値", padW, padH)
	}

	// 枠と見出しを除いた領域にちょうど収まる本文は、切り詰められずに全行出る。
	body := make([]string, 0, height-padH)
	for i := range height - padH {
		body = append(body, "行"+strconv.Itoa(i))
	}
	got := template.Modal(template.ModalInput{
		Title:  "見出し",
		Body:   strings.Join(body, "\n"),
		Width:  width,
		Height: height,
	})
	for _, line := range body {
		if !strings.Contains(got, line) {
			t.Errorf("本文の %q が切り詰められた:\n%s", line, got)
		}
	}

	// 枠と余白が使う幅も一致する（本文が幅いっぱいでも折り返さない）。
	filled := template.Modal(template.ModalInput{
		Title:  "見出し",
		Body:   strings.Repeat("x", width-padW),
		Width:  width,
		Height: height,
	})
	if w := lipgloss.Width(filled); w != width {
		t.Errorf("幅 = %d, want %d", w, width)
	}
}

// ちょうど内側幅に収まる本文は折り返さない。
//
// lipgloss の Width() は padding を含む幅なので、Modal が枠へ渡す幅と ModalPadding が
// 返す幅の区別を落とすと、page が配った本文がモーダル内で折り返す（詳細画面の操作
// リストが 2 行に割れる不具合）。幅を複数点で固定して再発を防ぐ。
func TestModalDoesNotWrapBodyAtInnerWidth(t *testing.T) {
	const height = 12
	padW, _ := template.ModalPadding()
	for _, width := range []int{60, 72, 80, 100} {
		t.Run(strconv.Itoa(width), func(t *testing.T) {
			inner := width - padW
			body := []string{
				strings.Repeat("x", inner),
				strings.Repeat("あ", inner/2),
				strings.Repeat("-", inner-1) + "x",
			}
			got := template.Modal(template.ModalInput{
				Title:  "見出し",
				Body:   strings.Join(body, "\n"),
				Width:  width,
				Height: height,
			})

			lines := strings.Split(got, "\n")
			if len(lines) != height {
				t.Errorf("行数 = %d, want %d", len(lines), height)
			}
			// 枠は領域を上下左右いっぱいに使う。Height() も枠と余白を含む大きさなので、
			// 内側の行数を渡すと枠が 2 行縮んで上下に空行が入る。
			if !strings.HasPrefix(lines[0], "╭") || !strings.HasPrefix(lines[len(lines)-1], "╰") {
				t.Errorf("枠が領域を埋めていない:\n%s", got)
			}
			for i, line := range lines {
				if w := lipgloss.Width(line); w > width {
					t.Errorf("%d 行目の表示幅 = %d, want <= %d", i+1, w, width)
				}
			}
			// 折り返すと本文の行に改行が入り、1 行として現れなくなる。
			for _, line := range body {
				if !strings.Contains(got, line) {
					t.Errorf("本文の %q が折り返している:\n%s", line, got)
				}
			}
		})
	}
}
