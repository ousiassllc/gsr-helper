package molecule

import (
	"time"

	"github.com/ousiassllc/gsr-helper/internal/ui/atom"
	"github.com/ousiassllc/gsr-helper/internal/ui/token"
)

// RunnerView は runner 1 行の描画に必要な値だけを落とした表示用の構造体。
type RunnerView struct {
	Name          string
	Scope         string
	Managed       string
	SvcActive     string        // systemctl の ActiveState
	SvcSub        string        // systemctl の SubState
	SvcUnknown    bool          // ユニットはあるが状態を取得できなかった
	Busy          bool          // ジョブ実行中か
	Elapsed       time.Duration // ジョブの経過時間
	Version       string
	LatestVersion string // 未取得なら空文字
	Work          string // 未集計は "-"（集計は internal/disk の担当）
	Warn          bool   // 行に注意事項があるか
}

// RunnerRow は cols と同じ順・同じ数のセルを返す。
//
// セル数を列数に必ず揃えるのは、bubbles/table が行のセルを走査しながら同じ添字の
// 列定義を引くため、セル数が列数を超えると添字範囲外で panic するからである。
func RunnerRow(v RunnerView, cols []token.Column, s token.Styles) []string {
	cells := make([]string, 0, len(cols))
	for _, c := range cols {
		cells = append(cells, runnerCell(v, c, s))
	}
	return cells
}

// runnerCell は列 1 つ分のセルを返す。
func runnerCell(v RunnerView, c token.Column, s token.Styles) string {
	switch c.ID {
	case token.ColName:
		return runnerNameCell(v, c, s)
	case token.ColScope:
		return dashCell(v.Scope, c, s)
	case token.ColManaged:
		return dashCell(v.Managed, c, s)
	case token.ColSvc:
		// 状態が取れなかったユニットは「ユニットなし」と書き分ける（atom.StatusUnknown）。
		if v.SvcUnknown {
			text, role := atom.StatusUnknown()
			return styledCell(text, role, c, s)
		}
		text, role := atom.StatusText(v.SvcActive, v.SvcSub)
		return styledCell(text, role, c, s)
	case token.ColJob:
		text, role := atom.JobText(v.Busy, v.Elapsed)
		return styledCell(text, role, c, s)
	case token.ColVersion:
		text, role := atom.VersionText(v.Version, v.LatestVersion)
		return styledCell(text, role, c, s)
	case token.ColWork:
		return dashCell(v.Work, c, s)
	default:
		// 知らない列でもセルを欠かさない（列数とセル数を必ず一致させる）。
		return atom.Cell("", c.Width, columnAlign(c))
	}
}

// runnerNameCell は名前の直後に注意記号を添えたセルを返す。
//
// 注意記号を行末（最後の列の後ろ）に足さないのは、bubbles/table が列幅を超えた
// 分を切り落とすため、行末に付けた記号が消えてしまうからである。名前の幅を
// 記号の分だけ狭めることで、記号の有無にかかわらず列幅が変わらない。
//
// 名前と記号を並べる余地が無い幅では名前を諦めて記号だけを描く。行に注意がある
// ことを示すのはこの記号だけなので、落とすと警告そのものが画面から消える。
// 名前が空のときは他の列（dashCell）と同じ「値なし」の記号を出す。空白のままだと
// 値が無いのか描画に失敗したのかを読み分けられない。
func runnerNameCell(v RunnerView, c token.Column, s token.Styles) string {
	const markWidth = 2 // 空白 + 注意記号

	if c.Width <= markWidth {
		if v.Warn {
			return atom.Pad(atom.WarnMark(v.Warn, s), c.Width, atom.Left)
		}
		return dashCell(v.Name, c, s)
	}

	name, role := v.Name, token.RolePlain
	if name == "" {
		name, role = token.IconNoUnit, token.RoleMuted
	}
	// 幅を揃えてから装飾する（styledCell と同じ順序。逆にすると切り詰めで ANSI 列が
	// 壊れる）。
	body := s.Style(role).Render(atom.Truncate(name, c.Width-markWidth))
	return atom.Pad(body+" "+atom.WarnMark(v.Warn, s), c.Width, atom.Left)
}
