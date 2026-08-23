package logs

import (
	"strings"

	dlogs "github.com/ousiassllc/gsr-helper/internal/logs"
	"github.com/ousiassllc/gsr-helper/internal/runner"
	"github.com/ousiassllc/gsr-helper/internal/ui/keymap"
	"github.com/ousiassllc/gsr-helper/internal/ui/molecule/listrow"
	"github.com/ousiassllc/gsr-helper/internal/ui/organism/table"
	"github.com/ousiassllc/gsr-helper/internal/ui/token"
)

// sectionLogs はログファイルの区画の添字。Logs タブは 1 区画だけを持つ。
const sectionLogs = 0

// row はファイル一覧の 1 行。ログ 1 件と、それを書いた runner の組。
//
// runner を持つのは、`journalctl` の対象（UnitName）と本文の見出しに出す名前が
// runner 側にあるためである。ログの列挙は runner 1 台ぶんずつ行い（dlogs.List）、
// ここで横断の一覧へ平坦化する。
type row struct {
	runner runner.Runner
	file   dlogs.File
}

// newTable はファイル一覧を組み立てる。
//
// 選択（チェックボックス）は持たない。Logs タブに一括操作は無く、対象は常に
// カーソル位置の 1 件だからである（Jobs タブと同じ理由）。
func newTable(keys keymap.Set, s token.Styles) table.Model[row] {
	return table.New(keys.List, s, table.SectionInput[row]{
		Title:      "",
		Columns:    token.LogColumns(),
		Rules:      token.LogColumnRules(),
		Render:     renderLog,
		ID:         func(r row) string { return r.file.Path },
		Match:      matchLog,
		Disabled:   nil,
		Selectable: false,
	})
}

// renderLog はログの行をセル列に変換する。
func renderLog(in table.RowInput[row]) []string {
	return listrow.LogRow(logView(in.Item), in.Cols, in.Styles)
}

// matchLog は絞り込みの一致判定。runner 名とファイル名を対象にする。
//
// 一覧の絞り込み（`/`）は Logs タブでは本文のフィルタに割り当てているため、この版で
// この判定を通る経路は無い。**それでも定義しておく。** table.SectionInput は Match を
// 必須とし、nil を渡すと絞り込みが始まった瞬間に落ちる。キーの割り当てが変わった
// ときに、判定が無いことが実行時の panic として現れる形にしない。
func matchLog(r row, q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(strings.ToLower(r.runner.Name()), q) ||
		strings.Contains(strings.ToLower(r.file.Name), q)
}

// logView はログを表示用の構造体に落とす。
func logView(r row) listrow.LogView {
	return listrow.LogView{
		Runner:  r.runner.Name(),
		Name:    r.file.Name,
		Size:    r.file.Size,
		Updated: r.file.ModTime,
	}
}

// logRows は runner ごとのログを 1 つの一覧へ平坦化し、更新時刻の降順に並べる。
//
// runner をまたいで時刻順に並べるのは、利用者が見たいのが「直近に動いたログ」だから
// である（FR-23）。runner ごとに区切ると、どの区画の先頭が最新かを目で追うことになる。
func logRows(runners []runner.Runner) []row {
	var out []row
	for _, r := range runners {
		files, err := dlogs.List(r)
		if err != nil {
			// 1 台の `_diag` が読めなくても一覧全体は出す。読めない runner の行が
			// 消えるだけであり、他の runner のログを見る妨げにはならない。
			continue
		}
		for _, f := range files {
			out = append(out, row{runner: r, file: f})
		}
	}
	sortByNewest(out)
	return out
}
