package listrow

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/ousiassllc/gsr-helper/internal/ui/token"
)

// 接頭辞ごとに割り当てる役割が変わる（追加 = OK 色 / 削除 = 失敗色 / 変更なし = 素）。
//
// 色付きのスタイルで比べるのは、役割の取り違え（追加と削除が同じ色になる、
// 文脈行まで色が付く）を色なしの期待値では検出できないためである。
func TestDiffLineRoles(t *testing.T) {
	colored := token.NewStyles(true, true)
	tests := map[string]struct {
		line string
		want string
	}{
		"追加は OK 色":   {"+ https_proxy=http://new-proxy:3128", colored.OK.Render("+ https_proxy=http://new-proxy:3128")},
		"削除は失敗色":     {"- https_proxy=http://old-proxy:3128", colored.Fail.Render("- https_proxy=http://old-proxy:3128")},
		"変更なしは装飾しない": {"  LANG=ja_JP.UTF-8", "  LANG=ja_JP.UTF-8"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if got := DiffLine(tt.line, 0, colored); got != tt.want {
				t.Errorf("DiffLine(%q) = %q, want %q", tt.line, got, tt.want)
			}
		})
	}
}

// 接頭辞の付いていない行や空行でも panic せず、本文をそのまま返す。
//
// 差分の作り手が変わっても画面が壊れないようにするための契約である。判定できない
// 行を色付けすると、追加でも削除でもない行が変更に見える。
func TestDiffLineWithoutPrefix(t *testing.T) {
	colored := token.NewStyles(true, true)

	for _, line := range []string{"", " ", "+", "-", "PATH=/usr/bin", "± 両方", "---", token.IconEllipsis} {
		if got := DiffLine(line, 0, colored); got != line {
			t.Errorf("DiffLine(%q) = %q, want 装飾なしの本文", line, got)
		}
	}
}

// 色を無効にした設定では、どの接頭辞でも本文だけを返す（期待値に ANSI 列が混ざらない）。
func TestDiffLineWithoutColor(t *testing.T) {
	for _, line := range []string{"+ a=1", "- a=0", "  a=0"} {
		if got := DiffLine(line, 0, plainStyles()); got != line {
			t.Errorf("色なしの差分行 = %q, want %q", got, line)
		}
	}
}

// 幅を超える行は切り詰める。装飾を含めても幅を超えない。
//
// ダイアログは幅を超える行を出してはならない。超えた行はモーダルの枠の中で
// 折り返し、枠の下辺が領域の外へ押し出される（template.Modal）。
func TestDiffLineFitsWidth(t *testing.T) {
	colored := token.NewStyles(true, true)
	const line = "+ ACTIONS_RUNNER_HOOK_JOB_COMPLETED=/opt/hooks/cleanup.sh"

	for _, width := range []int{1, 4, 20, 56, 80} {
		got := DiffLine(line, width, colored)
		if w := lipgloss.Width(got); w > width {
			t.Errorf("幅 %d の行の幅 = %d（%q）", width, w, got)
		}
	}

	// 幅が未設定（0 以下）なら切り詰めない。
	for _, width := range []int{0, -1} {
		if got := DiffLine(line, width, plainStyles()); got != line {
			t.Errorf("幅 %d の行 = %q, want %q", width, got, line)
		}
	}

	// 全角を含む行でも幅の途中で切らない（切り詰めは atom.Truncate に任せる）。
	if got := DiffLine("- ラベル=self-hosted,gpu", 10, plainStyles()); !strings.HasPrefix(got, "- ") {
		t.Errorf("切り詰めた行 = %q, want 接頭辞 %q を残す", got, "- ")
	}
}
