package table_test

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/ui/keymap"
	"github.com/ousiassllc/gsr-helper/internal/ui/molecule"
	"github.com/ousiassllc/gsr-helper/internal/ui/organism/table"
	"github.com/ousiassllc/gsr-helper/internal/ui/token"
)

// molecule の *Row 関数は RowInput を開く 1 行の関数で RenderRow に渡せる
// （RenderRow の doc の担保。page/<tab> の render* も同じ形をしている）。
var (
	_ table.RenderRow[molecule.RunnerView] = func(in table.RowInput[molecule.RunnerView]) []string {
		return molecule.RunnerRow(in.Item, in.Cols, in.Styles)
	}
	_ table.RenderRow[molecule.JobView] = func(in table.RowInput[molecule.JobView]) []string {
		return molecule.JobRow(in.Item, in.Cols, in.Styles)
	}
	_ table.RenderRow[molecule.OrphanView] = func(in table.RowInput[molecule.OrphanView]) []string {
		return molecule.OrphanRow(in.Item, in.Cols, in.Styles)
	}
)

// press はキー入力の Msg を作る。文字キーは Text、特殊キーは Code で表す
// （bubbletea v2 の Key.String は Text があればそれを、無ければ keystroke を返す）。
//
// 矢印やページキーも Code で表すのは、実端末が送るキー（Text は空）と同じ形にするため
// である。Text に "down" を入れると、入力モードでは文字入力になって検証にならない。
func press(k string) tea.KeyPressMsg {
	special := map[string]rune{
		"space": tea.KeySpace, "enter": tea.KeyEnter, "esc": tea.KeyEscape, "tab": tea.KeyTab,
		"up": tea.KeyUp, "down": tea.KeyDown, "left": tea.KeyLeft, "right": tea.KeyRight,
		"pgup": tea.KeyPgUp, "pgdown": tea.KeyPgDown, "home": tea.KeyHome, "end": tea.KeyEnd,
	}
	if code, ok := special[k]; ok {
		return tea.KeyPressMsg{Code: code}
	}
	if rest, ok := strings.CutPrefix(k, "ctrl+"); ok {
		return tea.KeyPressMsg{Code: rune(rest[0]), Mod: tea.ModCtrl}
	}
	return tea.KeyPressMsg{Text: k, Code: []rune(k)[0]}
}

// testStyles は色を使わないスタイル。期待文字列に ANSI 列が混ざらないようにする。
func testStyles() token.Styles {
	return token.NewStyles(true, false)
}

// row は一覧の行を表すテスト用の型。Table[T] の T は呼び出し側が決める。
type row struct {
	name string
	note string
}

// runnerSection は runner の一覧に相当する区画を返す。
func runnerSection(selectable bool) table.SectionInput[row] {
	return table.SectionInput[row]{
		Title: "",
		Columns: []token.Column{
			{ID: token.ColName, Title: "NAME", Width: 12},
			{ID: token.ColNote, Title: "NOTE", Width: 10},
		},
		Render:     renderRow,
		ID:         func(r row) string { return r.name },
		Match:      func(r row, q string) bool { return strings.Contains(r.name, q) },
		Disabled:   nil,
		Selectable: selectable,
	}
}

// orphanSection は孤児ユニットに相当する区画を返す。選択も絞り込みもしない。
func orphanSection() table.SectionInput[row] {
	return table.SectionInput[row]{
		Title:      "孤児ユニット",
		Columns:    []token.Column{{ID: token.ColUnit, Title: "UNIT", Width: 20}},
		Render:     renderRow,
		ID:         func(r row) string { return r.name },
		Match:      nil,
		Disabled:   nil,
		Selectable: false,
	}
}

// renderRow は列数ぶんのセルを返す（molecule の *Row 関数と同じ約束）。
//
// 最終セルに選択不可の理由を載せるのは、Disk タブが取る形と同じである
// （理由をどの列に置くかは Render の判断。RowInput.Reason の doc を参照）。
func renderRow(in table.RowInput[row]) []string {
	cells := make([]string, len(in.Cols))
	for i := range cells {
		cells[i] = in.Item.note
	}
	if len(cells) > 0 {
		cells[0] = in.Item.name
	}
	if in.Reason != "" && len(cells) > 1 {
		cells[len(cells)-1] = in.Reason
	}
	return cells
}

// rows は名前だけの行を n 件返す。
func rows(names ...string) []row {
	out := make([]row, 0, len(names))
	for _, n := range names {
		out = append(out, row{name: n, note: "active"})
	}
	return out
}

// newTable は 1 区画の一覧を組み立てる。
func newTable(selectable bool, items []row) table.Model[row] {
	t := table.New(keymap.NewList(), testStyles(), runnerSection(selectable))
	t.SetSize(80, 12)
	t.SetItems(0, items)
	return t
}

// send はキーを順に送り、最後の Cmd を返す。
func send(t table.Model[row], keys ...string) (table.Model[row], tea.Cmd) {
	var cmd tea.Cmd
	for _, k := range keys {
		t, cmd = t.Update(press(k))
	}
	return t, cmd
}

// names は行の名前を並び順に返す。選択・表示・列の比較をすべて名前で行う。
func names(items []row) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, item.name)
	}
	return out
}

// columnIDs は列の識別子を並び順に返す。
func columnIDs(cols []token.Column) []string {
	out := make([]string, 0, len(cols))
	for _, c := range cols {
		out = append(out, c.ID)
	}
	return out
}

// selectedName はカーソル位置の行の名前を返す。行が無ければ空文字を返す。
func selectedName(t table.Model[row]) string {
	item, ok := t.Selected()
	if !ok {
		return ""
	}
	return item.name
}
