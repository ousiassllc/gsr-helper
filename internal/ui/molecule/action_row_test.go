package molecule

import (
	"strconv"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/ousiassllc/gsr-helper/internal/ui/atom"
	"github.com/ousiassllc/gsr-helper/internal/ui/token"
)

// 実行できない操作はキーを消さず、理由を右に出す。
//
// 色を使えない端末でも、同じ行の右端に出る理由が有効な操作との違いになる
// （screens.md の設計原則 4）。
func TestActionRowEnabledAndDisabled(t *testing.T) {
	const width = 60
	if got, want := ActionRow(ActionView{Key: "l", Desc: "ログを開く", Enabled: true}, width, plainStyles()), "l:ログを開く"; got != want {
		t.Errorf("有効な操作の ActionRow = %q, want %q", got, want)
	}

	v := ActionView{Key: "s", Desc: "開始", Reason: "（稼働中のため不要）", Enabled: false}
	got := ActionRow(v, width, plainStyles())

	if !strings.HasPrefix(got, "s:開始") {
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

// 実行できない操作は影響を併記せず、理由だけを出す。
//
// 実行できない操作の影響は起こらないため併記する意味が無く、利用者に必要なのは
// 「なぜ押せないのか」である。両方を出すと幅 80 で行が折り返し、理由が読めなくなる
// （screens.md の詳細画面のモックにも影響と理由が同時に出る行は無い）。
func TestActionRowDisabledHidesImpact(t *testing.T) {
	v := ActionView{
		Key:         "X",
		Desc:        "強制停止",
		Impact:      token.IconWarn + " 実行中のジョブは中断されます",
		Reason:      "root 権限が必要です（sudo で起動してください）",
		Enabled:     false,
		Destructive: true,
	}
	const width = 74 // 幅 80 の端末で操作リストの行に配られる幅

	got := ActionRow(v, width, plainStyles())
	if strings.Contains(got, v.Impact) {
		t.Errorf("無効な操作の ActionRow = %q, 影響が出ている", got)
	}
	if !strings.HasPrefix(got, "X:強制停止") || !strings.HasSuffix(got, v.Reason) {
		t.Errorf("無効な操作の ActionRow = %q, キーと説明・理由が揃っていない", got)
	}

	// 有効なら影響を出す（理由は出さない）。
	enabled := v
	enabled.Enabled = true
	if got := ActionRow(enabled, width, plainStyles()); !strings.Contains(got, v.Impact) {
		t.Errorf("有効な操作の ActionRow = %q, 影響が出ていない", got)
	}
}

// 幅が足りなくても理由を落とさず、panic しない。
//
// 収まらない分は理由の末尾を中略する。理由を丸ごと消さないのは atom.Justify と
// 同じ意図であり、キーと説明を残すのは「押せない操作」と「存在しない操作」を
// 区別できるようにするためである（screens.md の無効な操作の表示）。
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
		if w := lipgloss.Width(got); w > max(width, 0) {
			t.Errorf("幅 %d: 行の表示幅 = %d（%q）", width, w, got)
		}
		if width < 40 {
			continue
		}
		if !strings.HasPrefix(got, "D:削除") {
			t.Errorf("幅 %d: キーと説明が消えた: %q", width, got)
		}
		// 全文が入らない幅では中略記号付きで残る（空にはしない）。
		if !strings.Contains(got, v.Reason) && !strings.Contains(got, token.IconEllipsis) {
			t.Errorf("幅 %d: 理由が消えた: %q", width, got)
		}
	}
}

// screens.md の詳細画面のモックに出る全操作 × 可否の組み合わせが幅に収まる。
//
// モーダルの中で折り返すと理由が読めなくなる（page/overlay_test.go の
// TestOverlayDetailDoesNotWrapInModal と同じ不具合）。行を組む側でも幅を固定する。
func TestActionRowFitsWidth(t *testing.T) {
	// 端末幅から操作リストの 1 行に配られる幅。内訳はモーダルの枠と左右余白 4
	// （template.ModalPadding）+ 行頭のカーソルと空白 2（organism.cursorWidth）。
	const chrome = 6

	for _, term := range []int{60, 72, 80, 100} {
		t.Run(strconv.Itoa(term), func(t *testing.T) {
			width := term - chrome
			for _, a := range detailActions() {
				enabled := a
				enabled.Enabled = true
				assertRowFits(t, enabled, width, "")

				for _, reason := range detailReasons() {
					disabled := a
					disabled.Reason = reason
					assertRowFits(t, disabled, width, reason)
				}
			}
		})
	}
}

// 無効な行のキーと説明の表記は atom.KeyHint と同じである。
//
// 無効な行だけは装飾前の文字列から組む（幅に収める中略のため）ので、表記が
// 2 箇所に分かれる。同じ文字列になることを固定して食い違いを防ぐ。
func TestActionRowLabelMatchesKeyHint(t *testing.T) {
	for _, v := range []ActionView{
		{Key: "s", Desc: "開始"},
		{Key: "", Desc: "そのまま反映する"},
		{Key: "s", Desc: ""},
	} {
		hint := atom.KeyHint(atom.Hint{Key: v.Key, Desc: v.Desc, Enabled: false, Reason: ""}, plainStyles())
		if got := ActionRow(v, 60, plainStyles()); got != hint {
			t.Errorf("無効な行 = %q, want %q（atom.KeyHint と同じ表記）", got, hint)
		}
	}
}

// assertRowFits は行が幅に収まり、キーと理由が残ることを検証する。
func assertRowFits(t *testing.T, v ActionView, width int, reason string) {
	t.Helper()

	got := ActionRow(v, width, plainStyles())
	if w := lipgloss.Width(got); w > width {
		t.Errorf("%q の行の表示幅 = %d, want <= %d（%q）", v.Key, w, width, got)
	}
	if !strings.HasPrefix(got, v.Key) {
		t.Errorf("%q の行にキーが無い: %q", v.Key, got)
	}
	if reason != "" && !strings.Contains(got, reason) && !strings.Contains(got, token.IconEllipsis) {
		t.Errorf("%q の行から理由が消えた: %q", v.Key, got)
	}
}

// detailActions は screens.md の詳細画面のモックに出る操作を並び順で返す。
func detailActions() []ActionView {
	return []ActionView{
		{Key: "l", Desc: "ログを開く"},
		{Key: "s", Desc: "開始"},
		{Key: "d", Desc: "ドレイン停止"},
		{Key: "R", Desc: "再起動"},
		{Key: "E", Desc: "enable/disable の切替"},
		{Key: "e", Desc: "設定を編集"},
		{Key: "u", Desc: "バージョン更新"},
		{Key: "x", Desc: "停止", Destructive: true},
		{
			Key: "X", Desc: "強制停止", Destructive: true,
			Impact: token.IconWarn + " 実行中のジョブは中断されます",
		},
		{
			Key: "D", Desc: "削除", Destructive: true,
			Impact: token.IconWarn + " 登録解除 + サービス削除",
		},
	}
}

// detailReasons は screens.md「無効な操作の表示」の理由を返す。
func detailReasons() []string {
	return []string{
		"root 権限が必要です（sudo で起動してください）",
		"サービス制御は利用できません（systemctl が見つかりません）",
		"systemd 管理外のため操作できません",
		"GitHub の認証が必要です（gh auth login）",
		"ジョブ実行中です。先に d でドレイン停止してください",
		"この版では未対応です",
	}
}
