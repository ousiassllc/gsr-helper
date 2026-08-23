package molecule

import (
	"github.com/ousiassllc/gsr-helper/internal/ui/atom"
	"github.com/ousiassllc/gsr-helper/internal/ui/token"
)

const (
	// fsLabel は行の先頭に置く見出し。
	fsLabel = "ファイルシステム"
	// fsWarnNote は閾値を超えたときに行末へ出す文言。
	fsWarnNote = "警告閾値超過"
)

// FSSummaryView はファイルシステムの要約 1 行の表示用の構造体。
//
// 閾値そのものは持たない。超過したかの判断は設定（appconfig.DiskThresholds）と
// 実測値の両方を持つ page の責務であり、molecule は結果を描くだけにする。
type FSSummaryView struct {
	Path         string // マウントポイント（"/"）
	UsedPercent  int    // 使用率（0〜100）
	UsedBytes    int64  // 使用量。未取得は負の値
	TotalBytes   int64  // 総容量。未取得は 0 以下
	InodePercent int    // inode 使用率（0〜100）
	// Unavailable はファイルシステム情報そのものを取得できなかったか。
	//
	// 使用率は 0〜100 の範囲しか取れず、負値で「不明」を表せない（atom.Ratio は
	// 負値を 0 に丸める）。真偽値を別に持たないと、取得に失敗した状態がそのまま
	// 「使用 0% / inode 0%」になり、**枯渇しているのに潤沢に見える**。
	Unavailable bool
	Warn        bool // 警告閾値を超えたか
}

// FSSummaryLine は Disk タブの先頭に出すファイルシステムの要約を返す
// （screens.md の Disk タブ）。
//
// 記号（⚠）を使用率そのものではなく行末の文言に添えるのは、1 行に同じ意味の記号を
// 2 つ並べないためである。atom.Ratio は記号を付ける形も返せるが、この行では
// 「何が閾値を超えたのか」を文言で示す方が読みやすく、色を使えない端末でも
// 「警告閾値超過」の文字で判別できる（設計原則 4）。
//
// **この版では Warn は常に偽になる。** 閾値（appconfig.DiskThresholds）を page へ運ぶ
// 経路（page.StateMsg）がまだ無いためである。引数を今から持たせておくのは、閾値が
// 通るようになったときに呼び出し側だけの変更で済ませるためで、判断を molecule に
// 持ち込まない形（真偽値を受け取るだけ）は変わらない。
//
// マウントポイントの桁は固定しない。screens.md のモックは 16 セル取っているが、
// 下の一覧（TARGET 列）と桁を揃える相手がいない飾りの余白であり、埋めると警告付きの
// 行が保証する幅（80）に収まらない（モックの行は 83 セルある）。
//
// 幅に収まらない分は atom.Join が末尾から落とす。落ちるのは行末の警告であり、
// 警告を残してほかを落とす形にはしない。使用率と容量が消えた「警告だけの行」は
// 何が起きているのかを示せないためである。
func FSSummaryLine(v FSSummaryView, width int, s token.Styles) string {
	// 記号は行末の警告にまとめるので、使用率には添えない。
	const markOnRatio = false

	usedText, usedRole := fsRatio(v.Unavailable, v.UsedPercent, markOnRatio)
	inodeText, inodeRole := fsRatio(v.Unavailable, v.InodePercent, markOnRatio)

	parts := []string{
		fsLabel,
		fsPath(v.Path),
		"使用 " + s.Style(usedRole).Render(usedText) + fsCapacity(v),
		"inode " + s.Style(inodeRole).Render(inodeText),
	}
	if v.Warn {
		parts = append(parts, atom.WarnMark(true, s)+" "+s.Warn.Render(fsWarnNote))
	}
	return atom.Join(parts, "  ", width, token.IconEllipsis)
}

// fsRatio は使用率のセルを返す。取得できていない場合は「値なし」の記号にする。
//
// 0% と書き分けるためである。ファイルシステム情報の取得に失敗した行を「使用 0%」と
// 描くと、枯渇している相手を潤沢だと読ませる（internal/disk/fsstats.go が「最も
// 避けたい誤表示」として挙げた状態そのものである）。
func fsRatio(unavailable bool, percent int, warn bool) (string, token.RoleToken) {
	if unavailable {
		return token.IconNoUnit, token.RoleMuted
	}
	return atom.Ratio(percent, warn)
}

// fsPath はマウントポイントを返す。取得できていない場合は「値なし」の記号にする。
func fsPath(p string) string {
	if p == "" {
		return token.IconNoUnit
	}
	return p
}

// fsCapacity は使用量と総容量の併記を返す。総容量が未取得なら何も返さない。
//
// 未取得のときに "(-/-)" を出さないのは、括弧だけが残った表示が「容量 0」と
// 読めてしまうためである。使用率は df の報告値をそのまま出せるので、容量の
// 取得を待たずに行を描ける。
func fsCapacity(v FSSummaryView) string {
	if v.TotalBytes <= 0 {
		return ""
	}
	return " (" + atom.Bytes(v.UsedBytes) + "/" + atom.Bytes(v.TotalBytes) + ")"
}
