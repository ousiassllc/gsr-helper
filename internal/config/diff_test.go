package config_test

import (
	"strings"
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/config"
)

// 画面仕様「変更内容の確認」のモックどおりの印が付くこと。描画側
// （molecule/listrow.DiffLine）はこの 3 つの印だけで行の種類を見分けるため、
// 印が変わると差分の色分けが崩れる。
func TestDiffMarksLines(t *testing.T) {
	t.Parallel()

	before := strings.Join([]string{
		"PATH=/usr/local/bin:/usr/bin:/bin",
		"https_proxy=http://old-proxy:3128",
		"LANG=ja_JP.UTF-8",
	}, "\n") + "\n"
	after := strings.Join([]string{
		"PATH=/usr/local/bin:/usr/bin:/bin",
		"https_proxy=http://new-proxy:3128",
		"ACTIONS_RUNNER_HOOK_JOB_COMPLETED=/opt/hooks/cleanup.sh",
		"LANG=ja_JP.UTF-8",
	}, "\n") + "\n"

	want := strings.Join([]string{
		"  PATH=/usr/local/bin:/usr/bin:/bin",
		"- https_proxy=http://old-proxy:3128",
		"+ https_proxy=http://new-proxy:3128",
		"+ ACTIONS_RUNNER_HOOK_JOB_COMPLETED=/opt/hooks/cleanup.sh",
		"  LANG=ja_JP.UTF-8",
	}, "\n") + "\n"

	if got := config.Diff(before, after); got != want {
		t.Errorf("Diff() =\n%s\nwant\n%s", got, want)
	}
}

// 変更が無ければ文脈行だけになること。差分プレビューが「何も変わらない」ことを
// 示せる必要がある。
func TestDiffIdenticalHasOnlyContext(t *testing.T) {
	t.Parallel()

	got := config.Diff("A=1\nB=2\n", "A=1\nB=2\n")
	for _, l := range strings.Split(strings.TrimSuffix(got, "\n"), "\n") {
		if !strings.HasPrefix(l, config.MarkContext) {
			t.Errorf("変更なしなのに %q が文脈行でない", l)
		}
	}
}

// 末尾の改行の有無だけの違いは差分にしないこと。利用者に取れる行動が無い。
func TestDiffIgnoresTrailingNewlineOnly(t *testing.T) {
	t.Parallel()

	got := config.Diff("A=1", "A=1\n")
	if want := "  A=1\n"; got != want {
		t.Errorf("Diff() = %q, want %q", got, want)
	}
}

// 空との差分は全行の追加・削除になること。
func TestDiffWithEmpty(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		before, after string
		want          string
	}{
		"新規作成": {"", "A=1\nB=2\n", "+ A=1\n+ B=2\n"},
		"全削除":  {"A=1\nB=2\n", "", "- A=1\n- B=2\n"},
		"両方空":  {"", "", ""},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if got := config.Diff(tt.before, tt.after); got != tt.want {
				t.Errorf("Diff() = %q, want %q", got, tt.want)
			}
		})
	}
}

// コメントと空行も差分の対象になること。.env は全行を保持するため、
// コメントの変更も利用者に見せる必要がある。
func TestDiffCoversCommentsAndBlankLines(t *testing.T) {
	t.Parallel()

	got := config.Diff("# 古い見出し\n\nA=1\n", "# 新しい見出し\n\nA=1\n")
	want := "- # 古い見出し\n+ # 新しい見出し\n  \n  A=1\n"

	if got != want {
		t.Errorf("Diff() = %q, want %q", got, want)
	}
}

// 行数が上限を超えても固まらず、全行の入れ替えとして返すこと。
func TestDiffFallsBackOnHugeInput(t *testing.T) {
	t.Parallel()

	var before, after strings.Builder
	for i := range 1500 {
		before.WriteString("A=")
		before.WriteString(strings.Repeat("x", i%7))
		before.WriteString("\n")
		after.WriteString("B=")
		after.WriteString(strings.Repeat("y", i%7))
		after.WriteString("\n")
	}

	got := config.Diff(before.String(), after.String())
	if strings.Contains(got, config.MarkContext+"A=") {
		t.Error("上限超過では文脈行を作らないはず")
	}
	if !strings.HasPrefix(got, config.MarkRemove) {
		t.Errorf("削除行から始まっていない: %q", got[:20])
	}
}

// CRLF の行でも印が付き、改行コードが差分に紛れ込まないこと。
func TestDiffHandlesCRLF(t *testing.T) {
	t.Parallel()

	got := config.Diff("A=1\r\nB=2\r\n", "A=1\r\nB=3\r\n")
	want := "  A=1\n- B=2\n+ B=3\n"

	if got != want {
		t.Errorf("Diff() = %q, want %q", got, want)
	}
}
