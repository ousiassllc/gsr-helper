package molecule

import (
	"github.com/ousiassllc/gsr-helper/internal/ui/atom"
	"github.com/ousiassllc/gsr-helper/internal/ui/token"
)

// ActionView は詳細画面の操作リスト 1 行の表示用の構造体。
//
// 可否（Enabled）と不可の理由（Reason）は page が判断して渡す。molecule は
// 受け取った値を描くだけで、操作可否の判断を持たない。
type ActionView struct {
	Key         string // 押すキー
	Desc        string // 動作の説明
	Impact      string // 影響の併記（「⚠ 実行中のジョブは中断されます」など）
	Reason      string // 実行できない理由。Enabled が false のときのみ表示する
	Enabled     bool   // 実行できるか。false なら Reason を必ず埋める
	Destructive bool   // 破壊的な操作か（影響を警告色で描く）
}

// ActionRow は操作 1 行を返す。
//
// 区切り線（破壊的な操作を線の下にまとめる）は行の並べ方の問題であり、
// organism.ChoiceList の責務なのでここでは扱わない。
//
// 理由を出すのは実行できない場合だけにする。フッタ（KeyBar）と詳細画面の
// 操作リストで同じ理由を表示する（screens.md の無効な操作の表示）ため、
// 「理由を出す条件」を両者で揃える必要がある。
func ActionRow(v ActionView, width int, s token.Styles) string {
	left := atom.KeyHint(atom.Hint{
		Key:     v.Key,
		Desc:    v.Desc,
		Enabled: v.Enabled,
		Reason:  v.Reason,
	}, s)

	if v.Impact != "" {
		// 破壊的な操作の影響は警告色で描き、区切り線の下に置く行だと分かるようにする。
		impact := s.Warn.Render(v.Impact)
		if v.Destructive {
			impact = s.Danger.Render(v.Impact)
		}
		left += "（" + impact + "）"
	}
	if v.Enabled || v.Reason == "" {
		return left
	}
	// 実行できない理由は右側に出す。幅が足りなくても消さない。
	return atom.Justify(left, s.Muted.Render(v.Reason), width)
}
