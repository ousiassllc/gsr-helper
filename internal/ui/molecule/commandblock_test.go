package molecule

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
)

// 1 コマンド 1 行で字下げして並べる（screens.md の確認ダイアログ）。
func TestCommandBlock(t *testing.T) {
	got := CommandBlock([]string{
		"（ファイル削除: 上記パスの再帰削除）",
		"docker builder prune -f",
	}, 80, plainStyles())

	want := "  （ファイル削除: 上記パスの再帰削除）\n  docker builder prune -f"
	if got != want {
		t.Errorf("CommandBlock = %q, want %q", got, want)
	}
}

// コマンドが無ければ空文字を返す。呼び出し側が見出しごと省けるようにする。
func TestCommandBlockEmpty(t *testing.T) {
	tests := map[string][]string{
		"nil":    nil,
		"空のスライス": {},
		"空文字だけ":  {"", ""},
	}
	for name, commands := range tests {
		t.Run(name, func(t *testing.T) {
			if got := CommandBlock(commands, 80, plainStyles()); got != "" {
				t.Errorf("CommandBlock = %q, want 空文字", got)
			}
		})
	}
}

// 空の要素は行を作らない（字下げだけの空行はコマンドの区切りに見える）。
func TestCommandBlockSkipsEmptyCommands(t *testing.T) {
	got := CommandBlock([]string{"a", "", "b"}, 80, plainStyles())
	if n := len(strings.Split(got, "\n")); n != 2 {
		t.Errorf("行数 = %d, want 2（%q）", n, got)
	}
}

// 幅を超えるコマンドは折り返し、継続行を深く字下げする。
//
// 継続行を見分けられないと、次のコマンドなのか 1 本の続きなのかが読めない。
// 折り返した行も含めて幅に収まっていることを併せて見る。
func TestCommandBlockWrapsWithContinuationIndent(t *testing.T) {
	const width = 40
	cmd := "rm -rf /opt/runners/build01-1/_work/bar/very-long-directory-name/deeper"

	lines := strings.Split(CommandBlock([]string{cmd}, width, plainStyles()), "\n")
	if len(lines) < 2 {
		t.Fatalf("行数 = %d, want 2 以上（折り返していない）", len(lines))
	}
	if !strings.HasPrefix(lines[0], commandIndent) || strings.HasPrefix(lines[0], commandContinue) {
		t.Errorf("1 行目 = %q, want %q で始まる", lines[0], commandIndent)
	}
	for _, l := range lines[1:] {
		if !strings.HasPrefix(l, commandContinue) {
			t.Errorf("継続行 = %q, want %q で始まる", l, commandContinue)
		}
	}
	for _, l := range lines {
		if w := lipgloss.Width(l); w > width {
			t.Errorf("行幅 = %d, want %d 以下（%q）", w, width, l)
		}
		if strings.HasSuffix(l, " ") {
			t.Errorf("行末に余白が残っている（%q）", l)
		}
	}
}

// 極端に狭い幅でも panic せず、内容を落とさない。
//
// 表示不能と判断するのは template.Frame の責務であり、ここでは行を返し続ける。
func TestCommandBlockNarrowWidth(t *testing.T) {
	for _, width := range []int{0, 1, 4, 5} {
		got := CommandBlock([]string{"docker builder prune -f"}, width, plainStyles())
		if got == "" {
			t.Errorf("幅 %d で空文字が返った", width)
		}
	}
}

// マスクは exec 層の責務であり、ここでは何もしない（受け取った文字列をそのまま出す）。
func TestCommandBlockDoesNotMask(t *testing.T) {
	const masked = "gh auth login --with-token ***"
	if got := CommandBlock([]string{masked}, 80, plainStyles()); !strings.Contains(got, masked) {
		t.Errorf("CommandBlock = %q, want %q をそのまま含む", got, masked)
	}
}
