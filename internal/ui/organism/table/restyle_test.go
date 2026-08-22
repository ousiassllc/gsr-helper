package table_test

import (
	"slices"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/ousiassllc/gsr-helper/internal/ui/keymap"
	"github.com/ousiassllc/gsr-helper/internal/ui/organism/table"
	"github.com/ousiassllc/gsr-helper/internal/ui/token"
)

// Restyle は行・見出し・カーソル・チェックボックス・区切り線を新しい配色で描き直す。
//
// 端末の背景色は起動後に届き、切り替わることもある（tea.BackgroundColorMsg）。配色を
// 取り込んだきりにすると、濃色向けの薄い色が白背景に残って読めなくなる。
func TestRestyleAppliesNewPaletteToEveryPart(t *testing.T) {
	dark, light := token.NewStyles(true, true), token.NewStyles(false, true)

	tbl := table.New(keymap.NewList(), dark, runnerSection(true), orphanSection())
	tbl.SetSize(80, 12)
	tbl.SetItems(0, rows("build01-1", "build01-2"))
	tbl.SetItems(1, rows("old01"))
	tbl, _ = send(tbl, "space") // チェックボックスを出す

	before := tbl.View()
	for name, want := range paletteSamples(dark) {
		if !strings.Contains(before, want) {
			t.Fatalf("%s が濃色で出ていない（テストの前提が崩れている）:\n%q", name, before)
		}
	}

	tbl.Restyle(keymap.NewList(), light)

	after := tbl.View()
	for name, want := range paletteSamples(light) {
		if !strings.Contains(after, want) {
			t.Errorf("%s が淡色の配色にならない:\n%q", name, after)
		}
	}
	for name, stale := range paletteSamples(dark) {
		if strings.Contains(after, stale) {
			t.Errorf("%s に濃色向けの配色が残っている:\n%q", name, after)
		}
	}
}

// paletteSamples は一覧の各部が配色から作る文字列を返す。
//
// 部位ごとに引くのは、行だけ・枠だけが取り残される抜けを 1 本のテストで捕まえる
// ためである（Issue #28 は行・カーソル・チェックボックス・区切り線が同時に古かった）。
//
// 見出しと区切り線は幅いっぱいに埋めた文字列を描くため、中身ではなく装飾の開始列
// （SGR）で引く。中身を期待値に書くと列幅を変えるたびにテストが壊れる。
func paletteSamples(s token.Styles) map[string]string {
	return map[string]string{
		"カーソル":     s.Cursor.Render(token.IconCursor),
		"チェックボックス": s.Selected.Render(token.IconChecked),
		"見出し":      sgrPrefix(s.Header),
		"区切り線":     sgrPrefix(s.Divider),
	}
}

// sgrPrefix はスタイルが中身の前に置く装飾の開始列を返す。
func sgrPrefix(st lipgloss.Style) string {
	prefix, _, ok := strings.Cut(st.Render("x"), "x")
	if !ok {
		return ""
	}
	return prefix
}

// 再スタイルでカーソル位置・選択・絞り込み文字列・フォーカス中の区画は失われない。
//
// 3 秒ごとの再検出のたびに配色が配られるため、ここで状態が飛ぶと操作を選んでいる
// 途中でカーソルが先頭へ戻る。
func TestRestyleKeepsCursorSelectionAndFilter(t *testing.T) {
	tbl := table.New(keymap.NewList(), token.NewStyles(true, true), runnerSection(true), orphanSection())
	tbl.SetSize(80, 12)
	tbl.SetItems(0, rows("build01-1", "build01-2", "build01-3"))
	tbl.SetItems(1, rows("old01"))

	// 絞り込みを確定してからカーソルを動かし、選択も作る。
	tbl, _ = send(tbl, "/", "b", "u", "i", "l", "d", "enter")
	tbl, _ = send(tbl, "j", "space")

	wantSelected, wantChecked := selectedName(tbl), names(tbl.Checked())
	wantFilter, wantFocus := tbl.FilterValue(), tbl.FocusedSection()
	wantShown := names(tbl.Shown(0))
	if wantFilter == "" || len(wantChecked) == 0 || wantSelected == "" {
		t.Fatalf("前提が崩れている: filter=%q checked=%v selected=%q", wantFilter, wantChecked, wantSelected)
	}

	tbl.Restyle(keymap.NewList(), token.NewStyles(false, true))

	if got := selectedName(tbl); got != wantSelected {
		t.Errorf("カーソル位置 = %q, want %q", got, wantSelected)
	}
	if got := names(tbl.Checked()); !slices.Equal(got, wantChecked) {
		t.Errorf("選択 = %v, want %v", got, wantChecked)
	}
	if got := tbl.FilterValue(); got != wantFilter {
		t.Errorf("絞り込み文字列 = %q, want %q", got, wantFilter)
	}
	if got := tbl.FocusedSection(); got != wantFocus {
		t.Errorf("フォーカス中の区画 = %d, want %d", got, wantFocus)
	}
	if got := names(tbl.Shown(0)); !slices.Equal(got, wantShown) {
		t.Errorf("絞り込み後の行 = %v, want %v", got, wantShown)
	}
}
