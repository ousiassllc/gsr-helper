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
//
// キーは丸括弧で囲む（screens.md の共通レイアウトのフッタ 2 行目）。フッタ 1 行目は
// 幅の都合でグレーアウトしか使えないため、色以外の手がかりはこの行が担う。
func TestKeyBarGroupsReasons(t *testing.T) {
	hints := []atom.Hint{
		{Key: "s", Desc: "開始", Enabled: false, Reason: rootReason},
		{Key: "x", Desc: "停止", Enabled: false, Reason: rootReason},
		{Key: "X", Desc: "強制停止", Enabled: false, Reason: rootReason},
		{Key: "l", Desc: "ログ", Enabled: true, Reason: "出ないはずの理由"},
	}
	reason := strings.Split(KeyBar(hints, 120, plainStyles()), "\n")[1]

	if want := "(s)(x)(X): " + rootReason; !strings.Contains(reason, want) {
		t.Errorf("理由の行 = %q, want %q を含む", reason, want)
	}
	if strings.Count(reason, rootReason) != 1 {
		t.Errorf("同じ理由が複数回出ている: %q", reason)
	}
	if strings.Contains(reason, "出ないはずの理由") {
		t.Errorf("有効なキーの理由が出ている: %q", reason)
	}
}

// 理由が複数あるときも取り落とさず、1 行に並べる。多数のキーを塞ぐ理由が先に来る。
//
// フッタは 2 行に固定（template.ChromeHeight）なので理由ごとに行を増やせない。
// 理由を件数にまとめて捨てるとフッタから辿れなくなるため、1 行に並べて幅が尽きた
// 分だけを中略する。先頭を最も多くのキーを塞ぐ理由にするのは、能力不足
// （root / systemd / 認証）が複数のキーに一斉に効き、最も広く当てはまるためである。
func TestKeyBarShowsEveryReasonWidestFirst(t *testing.T) {
	const unsupported = "この版では未対応です"
	hints := []atom.Hint{
		{Key: "l", Desc: "ログ", Enabled: false, Reason: unsupported},
		{Key: "d", Desc: "ドレイン", Enabled: false, Reason: unsupported},
		{Key: "s", Desc: "開始", Enabled: false, Reason: rootReason},
		{Key: "x", Desc: "停止", Enabled: false, Reason: rootReason},
		{Key: "X", Desc: "強制", Enabled: false, Reason: rootReason},
	}

	// 幅が足りる端末ではどの理由も中略されない。
	reason := strings.Split(KeyBar(hints, 200, plainStyles()), "\n")[1]
	for _, want := range []string{"(s)(x)(X): " + rootReason, "(l)(d): " + unsupported} {
		if !strings.Contains(reason, want) {
			t.Errorf("理由の行 = %q, want %q を含む", reason, want)
		}
	}
	if !strings.HasPrefix(reason, "(s)(x)(X): "+rootReason) {
		t.Errorf("理由の行 = %q, 最も多くのキーを塞ぐ理由が先頭に無い", reason)
	}

	// 幅 80 では収まらないが、先頭の理由は最後まで残り、行数も増えない。
	lines := strings.Split(KeyBar(hints, token.WidthTarget, plainStyles()), "\n")
	if len(lines) != 2 {
		t.Fatalf("行数 = %d, want 2", len(lines))
	}
	if !strings.Contains(lines[1], rootReason) {
		t.Errorf("理由の行 = %q, 先頭の理由が中略されている", lines[1])
	}
	if w := lipgloss.Width(lines[1]); w > token.WidthTarget {
		t.Errorf("理由の行の幅 = %d, want %d 以下（%q）", w, token.WidthTarget, lines[1])
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
