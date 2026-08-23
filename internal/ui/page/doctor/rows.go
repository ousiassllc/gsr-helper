package doctor

import (
	"strings"

	"github.com/ousiassllc/gsr-helper/internal/doctor"
	"github.com/ousiassllc/gsr-helper/internal/ui/keymap"
	"github.com/ousiassllc/gsr-helper/internal/ui/molecule/listrow"
	"github.com/ousiassllc/gsr-helper/internal/ui/organism/table"
	"github.com/ousiassllc/gsr-helper/internal/ui/token"
)

// sectionResults は診断結果の区画の添字。Doctor タブは 1 区画だけを持つ。
const sectionResults = 0

// row は Doctor タブの 1 行。診断結果 1 件をそのまま持つ。
//
// 表示用の構造体へ落とさずドメインの型を持つのは、詳細画面（enter）と個別の
// 再実行（r）が識別子と対処の全文を要るためである。落とすのは molecule へ渡す
// 手前（rowView）で行う。
type row struct {
	result doctor.CheckResult
}

// newTable は Doctor タブの一覧を組み立てる。
func newTable(keys keymap.Set, s token.Styles) table.Model[row] {
	return table.New(keys.List, s, table.SectionInput[row]{
		Title:   "",
		Columns: token.DoctorColumns(),
		Rules:   token.DoctorColumnRules(),
		Render:  renderResult,
		ID:      rowID,
		Match:   matchResult,
		// 選択できないのは、Doctor タブに一括操作が無いためである。再実行の対象は
		// 全項目（r）かカーソル位置の 1 項目（詳細画面の r）のどちらかしかない。
		Disabled:   nil,
		Selectable: false,
	})
}

// rowID は行の識別子を返す。
//
// 識別子と対象を組にするのは、runner ごとに判定する項目（パーミッション・docker
// グループ所属）が同じ識別子で複数行を返すためである。識別子だけだと、再検出で
// 行を差し替えたときにカーソルが別の runner の行へ移る。
func rowID(r row) string { return r.result.ID + "\x00" + r.result.Target }

// renderResult は診断結果の行をセル列に変換する。
func renderResult(in table.RowInput[row]) []string {
	return listrow.DoctorRow(rowView(in.Item.result), in.Cols, in.Styles)
}

// rowView はドメインの型を表示用の構造体に落とす。
func rowView(r doctor.CheckResult) listrow.DoctorView {
	return listrow.DoctorView{
		Status:   stateToken(r.Status),
		Category: r.Category,
		Check:    r.Summary,
		Target:   r.Target,
	}
}

// matchResult は絞り込みの一致判定。
//
// 分類・要約・対象のいずれかに当たれば残す。「docker」と打って docker 関連の
// 項目を、runner 名を打ってその runner の項目を絞り込めるようにするためである。
func matchResult(r row, q string) bool {
	q = strings.ToLower(q)
	for _, field := range []string{r.result.Category, r.result.Summary, r.result.Target} {
		if strings.Contains(strings.ToLower(field), q) {
			return true
		}
	}
	return false
}

// stateToken はドメインの判定を表示のトークンに写す。
//
// **UI 側で 1 箇所に閉じる。** ドメイン（internal/doctor）に token を import させると
// 依存が逆向きになる（atomic-design.md の依存の規則）。写しをここに置くことで、
// 判定を足したときに直す場所がこの switch 1 つで済む。
func stateToken(st doctor.Status) token.StateToken {
	switch st {
	case doctor.OK:
		return token.StateOK
	case doctor.Warn:
		return token.StateWarn
	case doctor.Fail:
		return token.StateFail
	case doctor.Skip:
		return token.StateSkip
	default:
		return token.StateSkip
	}
}

// resultRows は診断結果を一覧の行に写す。
//
// 並べ替えはしない。doctor.Run が分類・識別子・対象で整列済みであり、ここで
// 並べ直すと整列の規則が 2 箇所に分かれる。
func resultRows(results []doctor.CheckResult) []row {
	out := make([]row, 0, len(results))
	for _, r := range results {
		out = append(out, row{result: r})
	}
	return out
}
