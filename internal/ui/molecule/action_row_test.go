package molecule

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/ousiassllc/gsr-helper/internal/ui/token"
)

// 実行できない操作はキーを消さず、理由を右に出す。
//
// キーと説明は丸括弧で囲む。グレーアウトだけでは色を使えない端末で有効な操作と
// 区別できない（screens.md の設計原則 4）。
func TestActionRowEnabledAndDisabled(t *testing.T) {
	const width = 60
	if got, want := ActionRow(ActionView{Key: "l", Desc: "ログを開く", Enabled: true}, width, plainStyles()), "l:ログを開く"; got != want {
		t.Errorf("有効な操作の ActionRow = %q, want %q", got, want)
	}

	v := ActionView{Key: "s", Desc: "開始", Reason: "（稼働中のため不要）", Enabled: false}
	got := ActionRow(v, width, plainStyles())

	if !strings.HasPrefix(got, "(s:開始)") {
		t.Errorf("ActionRow = %q, キーと説明が先頭に無い", got)
	}
	if !strings.HasSuffix(got, "（稼働中のため不要）") {
		t.Errorf("ActionRow = %q, 理由が右端に無い", got)
	}
	if w := lipgloss.Width(got); w != width {
		t.Errorf("ActionRow の幅 = %d, want %d（%q）", w, width, got)
	}

	// 理由は実行できない場合にだけ出す。フッタ（KeyBar）と詳細画面の操作リストで
	// 同じ理由を表示する（screens.md の無効な操作の表示）ため条件を揃える。
	enabled := v
	enabled.Enabled = true
	if got, want := ActionRow(enabled, width, plainStyles()), "s:開始"; got != want {
		t.Errorf("有効な操作の ActionRow = %q, want %q（理由は出さない）", got, want)
	}
}

// 影響は説明に併記する。
func TestActionRowImpact(t *testing.T) {
	v := ActionView{
		Key:         "X",
		Desc:        "強制停止",
		Impact:      token.IconWarn + " 実行中のジョブは中断されます",
		Enabled:     true,
		Destructive: true,
	}
	got := ActionRow(v, 80, plainStyles())
	if want := "X:強制停止（" + token.IconWarn + " 実行中のジョブは中断されます）"; got != want {
		t.Errorf("ActionRow = %q, want %q", got, want)
	}

	// 破壊的かどうかで影響の色が変わる（色が有効なとき）。
	s := token.NewStyles(true, true)
	safe := v
	safe.Destructive = false
	if ActionRow(v, 80, s) == ActionRow(safe, 80, s) {
		t.Error("破壊的な操作と安全な操作で影響の表示が同じである")
	}
}

// 幅が足りなくても理由を落とさず、panic しない。
func TestActionRowNarrowWidth(t *testing.T) {
	v := ActionView{
		Key:     "D",
		Desc:    "削除",
		Impact:  token.IconWarn + " 登録解除 + サービス削除",
		Reason:  "ジョブ実行中です。先に d でドレイン停止してください",
		Enabled: false,
	}
	for _, width := range []int{80, 40, 1, 0, -5} {
		got := ActionRow(v, width, plainStyles())
		for _, want := range []string{"(D:削除)", v.Reason} {
			if !strings.Contains(got, want) {
				t.Errorf("幅 %d: %q が消えた: %q", width, want, got)
			}
		}
	}
}
