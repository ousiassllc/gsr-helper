package listrow

import (
	"github.com/ousiassllc/gsr-helper/internal/ui/atom"
	"github.com/ousiassllc/gsr-helper/internal/ui/token"
)

// labelScanning は容量が確定していない対象の SIZE 列に出す文字列。
//
// 表記は screens.md の Disk タブのモックに合わせる。空欄にしないのは、集計が
// 非同期で判明した行から順に埋まる（FR-28）ため、「まだ出ていない」と「0 バイト」を
// 読み分ける必要があるからである。11 セルあり、SIZE 列の幅（token.DiskColumns）は
// この文字列で決まっている。
const labelScanning = "（集計中…）"

// labelScanFailed は集計に失敗した対象の SIZE 列に出す文字列。
//
// 集計中と別の記号にするのは、待てば埋まるのか、待っても埋まらないのかを
// 区別するためである（色に依存せず判別できるようにする。設計原則 4）。
const labelScanFailed = "失敗"

// DiskTargetView は削除候補 1 行の表示用の構造体。
//
// ドメインの型（disk.Target）は受け取らない。molecule 以下は表示に必要な値だけを
// 落とした構造体とプリミティブを受け取るという規則に従う（atomic-design.md の依存の規則）。
type DiskTargetView struct {
	Target   string // 対象の名前（"build01-1 / _work/bar" / "docker / build cache"）
	Bytes    int64  // 容量。負なら未確定として「値なし」を出す
	Files    int64  // ファイル数。負なら不明（docker の対象は件数を持たない）
	Path     string // 実体のパス。空なら「値なし」（docker の対象はパスを持たない）
	Scanning bool   // 集計中か
	Reason   string // 選択できない理由（FR-31）。空でなければ PATH 列に出す
	Failed   bool   // 集計に失敗したか
}

// DiskTargetRow は cols と同じ順・同じ数のセルを返す。
//
// セル数を列数に必ず揃える理由は RunnerRow と同じで、bubbles/table が行のセルを
// 走査しながら同じ添字の列定義を引くためである。
//
// **選択できない理由は PATH 列に載せる。** どのセルに載せるかを決めるのは行を返す
// この関数であり（organism/table は Render が返したセルを並べるだけ）、行末に足すと
// bubbles/table が列幅を超えた分を切り落として理由が消える。パスと理由が同時に
// 出ることは無い（選べない対象のパスを見せる意味は無く、理由の方が行の読み方を決める）。
func DiskTargetRow(v DiskTargetView, cols []token.Column, s token.Styles) []string {
	cells := make([]string, 0, len(cols))
	for _, c := range cols {
		cells = append(cells, diskCell(v, c, s))
	}
	return cells
}

// diskCell は列 1 つ分のセルを返す。
func diskCell(v DiskTargetView, c token.Column, s token.Styles) string {
	switch c.ID {
	case token.ColTarget:
		if v.Target == "" {
			return styledCell(token.IconNoUnit, token.RoleMuted, c, s)
		}
		return styledCell(v.Target, diskRole(v, token.RolePlain), c, s)
	case token.ColSize:
		text, role := diskSizeText(v)
		return styledCell(text, diskRole(v, role), c, s)
	case token.ColFiles:
		return styledCell(atom.Files(v.Files), diskRole(v, missingRole(v.Files < 0)), c, s)
	case token.ColPath:
		return diskPathCell(v, c, s)
	default:
		// 知らない列でもセルを欠かさない（列数とセル数を必ず一致させる）。
		return atom.Cell("", c.Width, columnAlign(c))
	}
}

// diskSizeText は SIZE 列の素の文字列と役割を返す。
//
// 失敗を集計中より先に見るのは、失敗した対象がもう集計中ではないためである。
// 両方が立っている状態（集計を打ち切った直後）で「集計中」を出すと、待てば
// 埋まるように読めてしまう。
func diskSizeText(v DiskTargetView) (text string, role token.RoleToken) {
	switch {
	case v.Failed:
		return token.Icon(token.StateFail) + " " + labelScanFailed, token.StateFail.Role()
	case v.Scanning:
		return labelScanning, token.RoleMuted
	default:
		return atom.Bytes(v.Bytes), missingRole(v.Bytes < 0)
	}
}

// diskPathCell は PATH 列のセルを返す。選択できない理由があればそれを載せる。
func diskPathCell(v DiskTargetView, c token.Column, s token.Styles) string {
	if v.Reason != "" {
		// 理由は補足情報なので薄く描く（孤児ユニットの NOTE と同じ扱い）。
		return styledCell(v.Reason, token.RoleMuted, c, s)
	}
	// パスは中間を中略して末尾（対象ごとに変わる部分）を残す。
	return dashCell(atom.Path(v.Path, c.Width), c, s)
}

// diskRole は行全体に効く役割を返す。
//
// 選択できない対象の行は薄く描く。チェックボックスが出ない行を通常の濃さで並べると、
// 選べるのに選び忘れている行に見える（screens.md の Disk タブのモックでは、この行だけ
// チェックボックスが無く行末に理由が出る）。
func diskRole(v DiskTargetView, base token.RoleToken) token.RoleToken {
	if v.Reason != "" {
		return token.RoleMuted
	}
	return base
}

// missingRole は値が無い場合に「値なし」と同じ薄さを返す。
//
// atom.Bytes / atom.Files が負の値へ返す記号を dashCell と同じ濃さで描くための
// 小さな橋渡しである。dashCell をそのまま使えないのは、記号を出すかどうかの判断が
// 空文字ではなく負の値だからである。
func missingRole(missing bool) token.RoleToken {
	if missing {
		return token.RoleMuted
	}
	return token.RolePlain
}
