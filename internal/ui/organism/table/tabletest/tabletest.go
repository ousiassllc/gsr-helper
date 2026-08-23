// Package tabletest は organism/table の検証で使う共通のフィクスチャを提供する。
//
// **参照は tabletest → table の一方向だけである。** `table` の非公開な状態は 1 つも
// export していない（使うのは `table.New` / `Model[T]` / `SectionInput[T]` /
// `RowInput[T]` / `RenderRow[T]` という既存の公開 API だけ）。したがって
// 「一覧の共通実装は 1 つ」（`Table` を増やさない）という規則は構造で守られたままで
// ある——`table` からこちらを指す辺は無く、`Model[T]` の内訳も分かれていない
// （atomic-design.md の「`ui/organism/table` を分割しない判断」）。
//
// **_test.go ではなく通常のパッケージに置く理由は行数上限である。** 行数チェックは
// 1 ディレクトリ 2000 行（テストを含む）を上限に直下のファイルだけを数えるため、
// `table` 直下の `helper_test.go` に置いたままではフィクスチャが本体と同じ予算を
// 食う。`page/pagetest` と同じ位置づけ・同じ理由の置き場である。
//
// **本番の経路からは import しない。** 通常のパッケージである以上 Go は止められない
// ので、`page/pagetest/import_test.go` の TestNoProductionCodeImportsTestFixtures が
// 各パッケージの本番ファイルの import を読んで検査する。
package tabletest

import (
	"regexp"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/ousiassllc/gsr-helper/internal/ui/keymap"
	"github.com/ousiassllc/gsr-helper/internal/ui/organism/table"
	"github.com/ousiassllc/gsr-helper/internal/ui/token"
)

// FilterPrompt は絞り込みの行の見出し。table パッケージの同名の非公開な定数と同じ値を
// 持つ（外から参照できないので写している。食い違えば配色の検証が前提から落ちる）。
const FilterPrompt = "絞り込み: "

// Press はキー入力の Msg を作る。文字キーは Text、特殊キーは Code で表す
// （bubbletea v2 の Key.String は Text があればそれを、無ければ keystroke を返す）。
//
// 矢印やページキーも Code で表すのは、実端末が送るキー（Text は空）と同じ形にするため
// である。Text に "down" を入れると、入力モードでは文字入力になって検証にならない。
func Press(k string) tea.KeyPressMsg {
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

// Styles は色を使わないスタイル。期待文字列に ANSI 列が混ざらないようにする。
func Styles() token.Styles { return token.NewStyles(true, false) }

// Row は一覧の行を表すテスト用の型。Table[T] の T は呼び出し側が決める。
type Row struct {
	Name string
	Note string
}

// RunnerSection は runner の一覧に相当する区画を返す。
func RunnerSection(selectable bool) table.SectionInput[Row] {
	return table.SectionInput[Row]{
		Title: "",
		Columns: []token.Column{
			{ID: token.ColName, Title: "NAME", Width: 12},
			{ID: token.ColNote, Title: "NOTE", Width: 10},
		},
		Render:     Render,
		ID:         func(r Row) string { return r.Name },
		Match:      func(r Row, q string) bool { return strings.Contains(r.Name, q) },
		Disabled:   nil,
		Selectable: selectable,
	}
}

// OrphanSection は孤児ユニットに相当する区画を返す。選択も絞り込みもしない。
func OrphanSection() table.SectionInput[Row] {
	return table.SectionInput[Row]{
		Title:      "孤児ユニット",
		Columns:    []token.Column{{ID: token.ColUnit, Title: "UNIT", Width: 20}},
		Render:     Render,
		ID:         func(r Row) string { return r.Name },
		Match:      nil,
		Disabled:   nil,
		Selectable: false,
	}
}

// Render は列数ぶんのセルを返す（molecule の *Row 関数と同じ約束）。
//
// 最終セルに選択不可の理由を載せるのは、Disk タブが取る形と同じである
// （理由をどの列に置くかは Render の判断。table.RowInput.Reason の doc を参照）。
func Render(in table.RowInput[Row]) []string {
	cells := make([]string, len(in.Cols))
	for i := range cells {
		cells[i] = in.Item.Note
	}
	if len(cells) > 0 {
		cells[0] = in.Item.Name
	}
	if in.Reason != "" && len(cells) > 1 {
		cells[len(cells)-1] = in.Reason
	}
	return cells
}

// Rows は名前だけの行を n 件返す。
func Rows(names ...string) []Row {
	out := make([]Row, 0, len(names))
	for _, n := range names {
		out = append(out, Row{Name: n, Note: "active"})
	}
	return out
}

// New は 1 区画の一覧を組み立てる。
func New(selectable bool, items []Row) table.Model[Row] {
	t := table.New(keymap.NewList(), Styles(), RunnerSection(selectable))
	t.SetSize(80, 12)
	t.SetItems(0, items)
	return t
}

// NewSectioned は Runners タブと同じ「一覧 + 孤児ユニット」の 2 区画を組み立てる。
func NewSectioned(runners, orphans []Row) table.Model[Row] {
	t := table.New(keymap.NewList(), Styles(), RunnerSection(true), OrphanSection())
	t.SetSize(80, 16)
	t.SetItems(0, runners)
	t.SetItems(1, orphans)
	return t
}

// NewColumned は与えた列を持つ 1 区画の一覧を組み立てる。r が nil なら既定の Render。
func NewColumned(cols []token.Column, selectable bool, r table.RenderRow[Row]) table.Model[Row] {
	sec := RunnerSection(selectable)
	sec.Columns = cols
	if r != nil {
		sec.Render = r
	}

	t := table.New(keymap.NewList(), Styles(), sec)
	t.SetSize(80, 12)
	t.SetItems(0, Rows("build01-1", "build01-2", "build02-1"))
	return t
}

// Send はキーを順に送り、最後の Cmd を返す。
func Send(t table.Model[Row], keys ...string) (table.Model[Row], tea.Cmd) {
	var cmd tea.Cmd
	for _, k := range keys {
		t, cmd = t.Update(Press(k))
	}
	return t, cmd
}

// Names は行の名前を並び順に返す。選択・表示・列の比較をすべて名前で行う。
func Names(items []Row) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, item.Name)
	}
	return out
}

// ColumnIDs は列の識別子を並び順に返す。
func ColumnIDs(cols []token.Column) []string {
	out := make([]string, 0, len(cols))
	for _, c := range cols {
		out = append(out, c.ID)
	}
	return out
}

// SelectedName はカーソル位置の行の名前を返す。行が無ければ空文字を返す。
func SelectedName(t table.Model[Row]) string {
	item, ok := t.Selected()
	if !ok {
		return ""
	}
	return item.Name
}

// ColumnSets は実際に使う列の組み合わせを返す。
func ColumnSets() map[string][]token.Column {
	return map[string][]token.Column{
		"Runner": token.RunnerColumns(),
		"Job":    token.JobColumns(),
		"Orphan": token.OrphanColumns(),
	}
}

// LongFilterKeys は絞り込みを始めて領域より長い文字列を打つキー列を返す。accept が真なら
// enter で確定する。1 文字で 2 桁使う全角で埋めるのは、検証する最大の幅（120）を短い
// キー列で超えるためである。
func LongFilterKeys(accept bool) []string {
	keys := []string{"/"}
	for range 70 {
		keys = append(keys, "長")
	}
	if accept {
		keys = append(keys, "enter")
	}
	return keys
}

// PaletteSamples は一覧の各部が配色から作る文字列を返す。部位ごとに引くのは、行だけ・枠だけが
// 取り残される抜けを 1 本で捕まえるためである（Issue #28 では 4 部位が同時に古かった）。見出しと
// 区切り線は幅いっぱいを埋めるので、中身ではなく装飾の開始列（SGR）で引く（中身を期待値に
// 書くと列幅を変えるたびに壊れる）。
func PaletteSamples(s token.Styles) map[string]string {
	return map[string]string{
		"カーソル":     s.Cursor.Render(token.IconCursor),
		"チェックボックス": s.Selected.Render(token.IconChecked),
		"見出し":      sgrPrefix(s.Header),
		"区切り線":     sgrPrefix(s.Divider),
		// 入力中の見出しは bubbles/textinput が Prompt のスタイルで描く。装飾の開始列ではなく
		// 装飾ごと引くのは、区切り線と同じ Muted を使うため開始列だけでは区別できないからである。
		"絞り込みプロンプト": s.Muted.Render(FilterPrompt),
	}
}

// sgrPrefix はスタイルが中身の前に置く装飾の開始列を返す。装飾が無ければ空文字になる。
func sgrPrefix(st lipgloss.Style) string {
	prefix, _, _ := strings.Cut(st.Render("x"), "x")
	return prefix
}

// SGRs は文字列に含まれる ANSI の装飾列（SGR）をすべて返す。色が入ったかを列単位で
// 調べるために使う。
func SGRs(s string) []string { return sgrSeq.FindAllString(s, -1) }

// sgrSeq は ANSI の装飾列（SGR）にあたる正規表現。
var sgrSeq = regexp.MustCompile("\x1b\\[[0-9;]*m")
