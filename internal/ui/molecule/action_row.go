package molecule

import (
	"github.com/ousiassllc/gsr-helper/internal/ui/atom"
	"github.com/ousiassllc/gsr-helper/internal/ui/token"
)

// ActionView は詳細画面の操作リスト 1 行の表示用の構造体。
//
// 可否（Enabled）と不可の理由（Reason）は page が判断して渡す。molecule は
// 受け取った値を描くだけで、操作可否の判断を持たない。
//
// **Impact は Enabled が真のときのみ、Reason は偽のときのみ表示する。**
// 実行できない操作の影響は起こらないため併記しない。利用者に必要なのは
// 「なぜ押せないのか」であり、両方を右側に並べると幅 80 で行が折り返して
// 理由が読めなくなる（screens.md の詳細画面のモックにも両方が出る行は無い）。
// どちらを出すかは幅に収めるための表示の都合なので、page ではなくここで選ぶ。
type ActionView struct {
	Key         string // 押すキー
	Desc        string // 動作の説明
	Impact      string // 影響の併記（「⚠ 実行中のジョブは中断されます」など）。Enabled が true のときだけ出す
	Reason      string // 実行できない理由。Enabled が false のときだけ出す
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
	if !v.Enabled {
		return disabledActionRow(v, width, s)
	}

	left := atom.KeyHint(atom.Hint{
		Key:     v.Key,
		Desc:    v.Desc,
		Enabled: true,
		Reason:  "",
	}, s)
	if v.Impact == "" {
		return left
	}
	// 破壊的な操作の影響は警告色で描き、区切り線の下に置く行だと分かるようにする。
	//
	// 影響を出す行は幅に収める処理をしない。影響はキーごとに決まった短い文言で、
	// 最長でも "X:強制停止（⚠ 実行中のジョブは中断されます）"（44 セル）であり、
	// 対応する最小幅 60 の端末（操作リストの行幅 54 セル）に収まる（screens.md の
	// 「幅 60 未満では表示不能」）。ここで中略すると、キー・説明・影響の 3 つを
	// 別々の色で描く行を装飾前に組み直すことになり、色の割り当てが崩れる。
	impact := s.Warn.Render(v.Impact)
	if v.Destructive {
		impact = s.Danger.Render(v.Impact)
	}
	return left + "（" + impact + "）"
}

// disabledActionRow は実行できない操作の行を返す。理由を右側に出す。
//
// **行全体を素の文字列で組んでから 1 度に装飾する。** 無効な行はキー・説明・理由の
// すべてを Muted で描く（atom.KeyHint の無効時と同じ）ため 1 つの装飾で足り、
// 装飾前なら幅に収める中略を atom に任せられる。装飾済みの文字列を切り詰めると
// ANSI 列が壊れる（atom.Truncate の契約）。
//
// 幅に収まらない場合に中略されるのは行の末尾＝理由の側である。キーと説明は左端に
// 残るため、「押せないキーを消さない」規則（screens.md の無効な操作の表示）を保つ。
// 理由を丸ごと空にはしないので atom.Justify の意図（幅の都合で理由を消さない）も
// 保たれる。理由の側を削るのは、幅 80 で最も長い組み合わせ（"E:enable/disable の切替"
// と「サービス制御は利用できません（systemctl が見つかりません）」＝ 82 セル）が
// 行幅 74 セルに収まらず、キーを消すと「押せない操作」と「存在しない操作」の区別が
// つかなくなるためである。
func disabledActionRow(v ActionView, width int, s token.Styles) string {
	row := atom.Justify(actionLabel(v.Key, v.Desc), v.Reason, width)
	return s.Muted.Render(atom.Truncate(row, width))
}

// actionLabel はキーと説明を装飾前の 1 つの文字列に組む。
//
// 表記は atom.KeyHint と揃える（"s:開始"。どちらかが空なら区切りを入れない）。
// KeyHint の戻り値は装飾済みで幅に収める中略ができないため、無効な行だけは
// ここで組む。表記が食い違わないことは action_row_test.go で固定する。
func actionLabel(key, desc string) string {
	if key == "" || desc == "" {
		return key + desc
	}
	return key + ":" + desc
}
