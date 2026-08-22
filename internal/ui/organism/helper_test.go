package organism_test

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/ui/keymap"
	"github.com/ousiassllc/gsr-helper/internal/ui/molecule"
	"github.com/ousiassllc/gsr-helper/internal/ui/organism"
	"github.com/ousiassllc/gsr-helper/internal/ui/token"
)

// molecule の *Row 関数は RenderRow としてそのまま渡せる（RenderRow の doc の担保）。
var (
	_ organism.RenderRow[molecule.RunnerView] = molecule.RunnerRow
	_ organism.RenderRow[molecule.JobView]    = molecule.JobRow
	_ organism.RenderRow[molecule.OrphanView] = molecule.OrphanRow
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
func runnerSection(selectable bool) organism.SectionInput[row] {
	return organism.SectionInput[row]{
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
func orphanSection() organism.SectionInput[row] {
	return organism.SectionInput[row]{
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
func renderRow(r row, cols []token.Column, _ token.Styles) []string {
	cells := make([]string, len(cols))
	for i := range cells {
		cells[i] = r.note
	}
	if len(cells) > 0 {
		cells[0] = r.name
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
func newTable(selectable bool, items []row) organism.Table[row] {
	t := organism.NewTable(keymap.NewList(), testStyles(), runnerSection(selectable))
	t.SetSize(80, 12)
	t.SetItems(0, items)
	return t
}

// send はキーを順に送り、最後の Cmd を返す。
func send(t organism.Table[row], keys ...string) (organism.Table[row], tea.Cmd) {
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
func selectedName(t organism.Table[row]) string {
	item, ok := t.Selected()
	if !ok {
		return ""
	}
	return item.name
}
