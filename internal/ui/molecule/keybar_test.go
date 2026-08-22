package molecule

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/ousiassllc/gsr-helper/internal/ui/atom"
	"github.com/ousiassllc/gsr-helper/internal/ui/token"
)

const rootReason = "root 権限が必要です（sudo で起動してください）"

// 理由の有無で高さを変えない（表示が上下に跳ねることを防ぐ）。
func TestKeyBarAlwaysReturnsTwoLines(t *testing.T) {
	cases := map[string][]atom.Hint{
		"理由なし":  {{Key: "s", Desc: "開始", Enabled: true}, {Key: "l", Desc: "ログ", Enabled: true}},
		"理由あり":  {{Key: "s", Desc: "開始", Enabled: false, Reason: rootReason}},
		"ヒントなし": nil,
	}
	for name, hints := range cases {
		lines := strings.Split(KeyBar(hints, token.WidthTarget, plainStyles()), "\n")
		if len(lines) != 2 {
			t.Errorf("%s: 行数 = %d, want 2", name, len(lines))
		}
	}
}

// ?:ヘルプ は KeyBar が付ける。呼び出し側の hints に含める運用ではない。
//
// フッタは常に ?:ヘルプ を含む（screens.md の共通レイアウト）。ちょうど収まる幅で
// 全ヒントが落ちて ?:ヘルプ だけが残ることも無い。
func TestKeyBarAlwaysShowsHelp(t *testing.T) {
	hints := []atom.Hint{
		{Key: "s", Desc: "開始", Enabled: true},
		{Key: "x", Desc: "停止", Enabled: true},
	}
	const want = "s:開始 x:停止 ?:ヘルプ"

	for _, w := range []int{token.WidthTarget, lipgloss.Width(want)} {
		lines := strings.Split(KeyBar(hints, w, plainStyles()), "\n")
		if lines[0] != want {
			t.Errorf("幅 %d のヒントの行 = %q, want %q", w, lines[0], want)
		}
		if lines[1] != "" {
			t.Errorf("幅 %d の理由の行 = %q, want 空行", w, lines[1])
		}
	}
	if lines := strings.Split(KeyBar(nil, token.WidthTarget, plainStyles()), "\n"); lines[0] != "?:ヘルプ" {
		t.Errorf("ヒントが無いときのヒントの行 = %q, want %q", lines[0], "?:ヘルプ")
	}
}

// 同じ理由のキーはまとめて 1 つの理由として出す。有効なキーの理由は出さない。
func TestKeyBarGroupsReasons(t *testing.T) {
	hints := []atom.Hint{
		{Key: "s", Desc: "開始", Enabled: false, Reason: rootReason},
		{Key: "x", Desc: "停止", Enabled: false, Reason: rootReason},
		{Key: "X", Desc: "強制停止", Enabled: false, Reason: rootReason},
		{Key: "l", Desc: "ログ", Enabled: true, Reason: "出ないはずの理由"},
		{Key: "D", Desc: "削除", Enabled: false, Reason: "GitHub の認証が必要です"},
	}
	reason := strings.Split(KeyBar(hints, 120, plainStyles()), "\n")[1]

	for _, want := range []string{"s/x/X: " + rootReason, "D: GitHub の認証が必要です"} {
		if !strings.Contains(reason, want) {
			t.Errorf("理由の行 = %q, want %q を含む", reason, want)
		}
	}
	if strings.Count(reason, rootReason) != 1 {
		t.Errorf("同じ理由が複数回出ている: %q", reason)
	}
	if strings.Contains(reason, "出ないはずの理由") {
		t.Errorf("有効なキーの理由が出ている: %q", reason)
	}
}

// 幅に収まらない分は ?:ヘルプ に集約し、理由の行も幅に収める。
func TestKeyBarFitsWidth(t *testing.T) {
	hints := []atom.Hint{
		{Key: "s", Desc: "開始", Enabled: true},
		{Key: "x", Desc: "停止", Enabled: true},
		{Key: "X", Desc: "強制停止", Enabled: true},
		{Key: "d", Desc: "ドレイン停止", Enabled: true},
		{Key: "D", Desc: "削除", Enabled: false, Reason: rootReason},
		{Key: "n", Desc: "追加", Enabled: false, Reason: "org レベルの操作には admin:org が必要です"},
		{Key: "l", Desc: "ログ", Enabled: true},
	}
	for _, width := range []int{token.WidthTarget, 40, 30, 10} {
		lines := strings.Split(KeyBar(hints, width, plainStyles()), "\n")
		for i, line := range lines {
			if w := lipgloss.Width(line); w > width {
				t.Errorf("幅 %d: %d 行目の幅 = %d（%q）", width, i+1, w, line)
			}
		}
		if !strings.Contains(lines[0], "?:ヘルプ") {
			t.Errorf("幅 %d: ?:ヘルプ が無い: %q", width, lines[0])
		}
		if got := strings.Count(lines[0], "?:ヘルプ"); got != 1 {
			t.Errorf("幅 %d: ?:ヘルプ が %d 回出ている: %q", width, got, lines[0])
		}
	}

	// 溢れても先頭のキーは残す。
	if lines := strings.Split(KeyBar(hints, 30, plainStyles()), "\n"); !strings.HasPrefix(lines[0], "s:開始") {
		t.Errorf("先頭のキーが落ちた: %q", lines[0])
	}
}
