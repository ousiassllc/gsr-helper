package molecule

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/ousiassllc/gsr-helper/internal/ui/token"
)

// screens.md の Setup タブのモックがそのまま出る。
//
// 記号・名前の桁・説明の並びを固定する。行ごとに説明の開始位置がずれると、
// 縦に並べたときにどれが同じ列なのか読めなくなる。
func TestProgressRowMatchesMock(t *testing.T) {
	const width = 78

	for _, tc := range []struct {
		name string
		v    ProgressView
		want string
	}{
		{
			name: "完了",
			v:    ProgressView{Name: "build01-5", State: ProgressDone, Detail: "登録・サービス起動 完了"},
			want: "✓ build01-5   登録・サービス起動 完了",
		},
		{
			name: "実行中",
			v:    ProgressView{Name: "build01-6", State: ProgressRunning, Detail: "config.sh 実行中…"},
			want: "▶ build01-6   config.sh 実行中…",
		},
		{
			name: "待機",
			v:    ProgressView{Name: "build01-7", State: ProgressWaiting, Detail: "待機"},
			want: "  build01-7   待機",
		},
		{
			name: "失敗",
			v:    ProgressView{Name: "build01-6", State: ProgressFailed, Detail: "失敗（config.sh 終了コード 1）"},
			want: "✗ build01-6   失敗（config.sh 終了コード 1）",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := ProgressRow(tc.v, width, plainStyles()); got != tc.want {
				t.Errorf("ProgressRow = %q, want %q", got, tc.want)
			}
		})
	}
}

// 状態ごとに違う記号が付く。色を使えない端末でも進み具合を読み分けられる
// （screens.md の設計原則 4）。
func TestProgressRowIconsDiffer(t *testing.T) {
	states := []ProgressState{ProgressWaiting, ProgressRunning, ProgressDone, ProgressFailed}

	seen := make(map[string]ProgressState, len(states))
	for _, st := range states {
		got := ProgressRow(ProgressView{Name: "r", State: st, Detail: ""}, 40, plainStyles())
		head, _, _ := strings.Cut(got+" ", " ")
		if prev, dup := seen[head]; dup {
			t.Errorf("状態 %d と %d の記号が同じ（%q）", prev, st, head)
		}
		seen[head] = st
	}

	// 未着手の記号は空白だが、桁は他の状態と揃う。
	waiting := ProgressRow(ProgressView{Name: "r", State: ProgressWaiting, Detail: "待機"}, 40, plainStyles())
	done := ProgressRow(ProgressView{Name: "r", State: ProgressDone, Detail: "待機"}, 40, plainStyles())
	if lipgloss.Width(waiting) != lipgloss.Width(done) {
		t.Errorf("未着手の行の幅 = %d, 完了の行 = %d（記号の有無で桁がずれている）",
			lipgloss.Width(waiting), lipgloss.Width(done))
	}
}

// 幅に収まらない行は末尾を中略し、幅を超えない。
//
// 進捗は 1 対象 1 行で読む表示なので、折り返して行数が増えるとどの行がどの対象なのか
// 追えなくなる。名前は残し、削れるのは説明の側である。
func TestProgressRowFitsWidth(t *testing.T) {
	v := ProgressView{
		Name:   "build01-12",
		State:  ProgressFailed,
		Detail: "失敗（config.sh 終了コード 1。Http response code: Forbidden）",
	}

	for _, width := range []int{78, 54, 20, 1, 0, -5} {
		got := ProgressRow(v, width, plainStyles())
		if w := lipgloss.Width(got); w > max(width, 0) {
			t.Errorf("幅 %d: 行の表示幅 = %d（%q）", width, w, got)
		}
		if width < 20 {
			continue
		}
		if !strings.Contains(got, v.Name) {
			t.Errorf("幅 %d: 名前が消えた: %q", width, got)
		}
		if !strings.Contains(got, v.Detail) && !strings.Contains(got, token.IconEllipsis) {
			t.Errorf("幅 %d: 説明が中略されずに消えた: %q", width, got)
		}
	}
}

// 説明が空でも行末に余白を残さない。
//
// 名前の桁を埋めた空白がそのまま残ると、選択・コピーのときに紛れ込む
// （molecule.CommandBlock が行末の余白を落とすのと同じ理由）。
func TestProgressRowTrimsTrailingSpace(t *testing.T) {
	got := ProgressRow(ProgressView{Name: "build01-5", State: ProgressDone, Detail: ""}, 40, plainStyles())
	if want := "✓ build01-5"; got != want {
		t.Errorf("ProgressRow = %q, want %q", got, want)
	}
}
