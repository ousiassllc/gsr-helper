package listrow

import (
	"strings"

	"github.com/ousiassllc/gsr-helper/internal/ui/atom"
	"github.com/ousiassllc/gsr-helper/internal/ui/token"
)

// 差分行の接頭辞。差分を作るドメイン層が付けたものをそのまま読む。
//
// 記号の後ろの空白まで含めて 2 セルで揃えるのは、行頭を縦にそろえて「変更なし」の
// 行と `-` / `+` の行を目で追えるようにするためである（screens.md の Config タブの
// 「変更内容の確認」）。
const (
	diffPrefixAdded   = "+ "
	diffPrefixRemoved = "- "
)

// DiffLine は Config タブの差分 1 行を装飾して返す。
//
// **受け取るのは接頭辞（"  " / "- " / "+ "）が付いた行そのものである。** 差分の計算も
// 行の解釈もここでは行わない。差分を作るのはドメイン層であり、UI 側でもう一度
// 突き合わせると、書き込む内容と画面に出る内容が食い違う経路ができる。役割の
// 判定は接頭辞だけで行う（molecule/log_line.go が重大度を判定せず役割を受け取るのと
// 同じ考え方だが、差分は接頭辞そのものが表示の一部なので行を丸ごと受け取る）。
//
// 追加を OK の色、削除を失敗の色に割り当てるのは、この 2 つが配色の中で最も
// 対になって見える組だからである。危険色（RoleDanger）を使わないのは、危険色が
// 「押すと戻せない操作」の警告に割り当ててあり（Confirm の影響欄）、差分の 1 行を
// それと同じ強さで描くと本当の警告が埋もれるためである。色を使えない端末では
// 接頭辞そのものが `-` / `+` を示すので、判別は色に依存しない（設計原則 4）。
//
// width を超える行は切り詰める。ダイアログは幅を超える行を出せない（超えた行は
// モーダルの枠の中で折り返し、枠の下辺が領域の外へ押し出される。template.Modal）。
// 切り詰めてから装飾するのは、装飾済みの文字列を切ると ANSI 列が壊れるためである
// （atom.Truncate の契約）。width が 0 以下なら幅は未設定として何もしない。
func DiffLine(line string, width int, s token.Styles) string {
	role := diffLineRole(line)
	if width > 0 {
		line = atom.Truncate(line, width)
	}
	if role == token.RolePlain {
		// 素通しのスタイルでも Render は文字列を作り直す。変更なしの行が差分の
		// 大半を占めるので、その 1 回を省く（molecule.LogLine と同じ）。
		return line
	}
	return s.Style(role).Render(line)
}

// diffLineRole は接頭辞から表示上の役割を返す。
//
// どちらの接頭辞でもない行（前後の文脈行や空行、接頭辞の付いていない行）は
// 装飾しない。判定できない行を色付けすると、追加でも削除でもない行が変更に見える。
func diffLineRole(line string) token.RoleToken {
	switch {
	case strings.HasPrefix(line, diffPrefixAdded):
		return token.RoleOK
	case strings.HasPrefix(line, diffPrefixRemoved):
		return token.RoleFail
	default:
		return token.RolePlain
	}
}
