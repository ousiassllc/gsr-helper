// Package filerow は Logs タブのファイル一覧の 1 行を組み立てる。
//
// page/logs 本体から分けているのは 2 つの理由による。
//   - ここにあるのは runner とログファイル（dlogs.File）を表示用の値へ落とす
//     **純粋関数**であり、tea.Model を組み立てずに検証できる。
//   - 1 ディレクトリ 2000 行の上限に対して page/logs に余裕が無い。Logs タブは
//     本文の組み立てと購読の 2 つを 1 つのタブに持つため page/<tab> のなかで最も
//     大きく、Issue #145 の追従で警告帯へ入った（Issue #147）。
//
// 先例は page/runners/rowview と page/disk/cleanview（どちらもドメインの値を
// 表示用の値へ落とす純粋関数である）。
package filerow

import (
	"slices"
	"strings"

	dlogs "github.com/ousiassllc/gsr-helper/internal/logs"
	"github.com/ousiassllc/gsr-helper/internal/runner"
	"github.com/ousiassllc/gsr-helper/internal/ui/keymap"
	"github.com/ousiassllc/gsr-helper/internal/ui/molecule/listrow"
	"github.com/ousiassllc/gsr-helper/internal/ui/organism/table"
	"github.com/ousiassllc/gsr-helper/internal/ui/token"
)

// Section はログファイルの区画の添字。Logs タブは 1 区画だけを持つ。
const Section = 0

// Row はファイル一覧の 1 行。ログ 1 件と、それを書いた runner の組。
//
// runner を持つのは、`journalctl` の対象（UnitName）と本文の見出しに出す名前が
// runner 側にあるためである。ログの列挙は runner 1 台ぶんずつ行い（dlogs.List）、
// ここで横断の一覧へ平坦化する。
type Row struct {
	// Runner はログを書いた runner。
	Runner runner.Runner
	// File はログファイル。
	File dlogs.File
}

// NewTable はファイル一覧を組み立てる。
//
// 選択（チェックボックス）は持たない。Logs タブに一括操作は無く、対象は常に
// カーソル位置の 1 件だからである（Jobs タブと同じ理由）。
func NewTable(keys keymap.Set, s token.Styles) table.Model[Row] {
	return table.New(keys.List, s, table.SectionInput[Row]{
		Title:      "",
		Columns:    token.LogColumns(),
		Rules:      token.LogColumnRules(),
		Render:     render,
		ID:         func(r Row) string { return r.File.Path },
		Match:      match,
		Disabled:   nil,
		Selectable: false,
	})
}

// render はログの行をセル列に変換する。
func render(in table.RowInput[Row]) []string {
	return listrow.LogRow(view(in.Item), in.Cols, in.Styles)
}

// match は絞り込みの一致判定。runner 名とファイル名を対象にする。
//
// 一覧の絞り込み（`/`）は Logs タブでは本文のフィルタに割り当てているため、この版で
// この判定を通る経路は無い。**それでも定義しておく。** table.SectionInput は Match を
// 必須とし、nil を渡すと絞り込みが始まった瞬間に落ちる。キーの割り当てが変わった
// ときに、判定が無いことが実行時の panic として現れる形にしない。
func match(r Row, q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(strings.ToLower(r.Runner.Name()), q) ||
		strings.Contains(strings.ToLower(r.File.Name), q)
}

// view はログを表示用の構造体に落とす。
func view(r Row) listrow.LogView {
	return listrow.LogView{
		Runner:  r.Runner.Name(),
		Name:    r.File.Name,
		Size:    r.File.Size,
		Updated: r.File.ModTime,
	}
}

// Rows は runner ごとのログを 1 つの一覧へ平坦化し、更新時刻の降順に並べる。
//
// runner をまたいで時刻順に並べるのは、利用者が見たいのが「直近に動いたログ」だから
// である（FR-23）。runner ごとに区切ると、どの区画の先頭が最新かを目で追うことになる。
func Rows(runners []runner.Runner) []Row {
	var out []Row
	for _, r := range runners {
		files, err := dlogs.List(r)
		if err != nil {
			// 1 台の `_diag` が読めなくても一覧全体は出す。読めない runner の行が
			// 消えるだけであり、他の runner のログを見る妨げにはならない。
			continue
		}
		for _, f := range files {
			out = append(out, Row{Runner: r, File: f})
		}
	}
	sortByNewest(out)
	return out
}

// sortByNewest は行を更新時刻の降順（同時刻はファイル名の降順）に並べる。
func sortByNewest(rows []Row) {
	slices.SortStableFunc(rows, func(a, b Row) int {
		if !a.File.ModTime.Equal(b.File.ModTime) {
			if a.File.ModTime.After(b.File.ModTime) {
				return -1
			}
			return 1
		}
		return strings.Compare(b.File.Name, a.File.Name)
	})
}
