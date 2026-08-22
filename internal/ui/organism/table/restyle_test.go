package table_test

import (
	"regexp"
	"slices"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/ousiassllc/gsr-helper/internal/ui/keymap"
	"github.com/ousiassllc/gsr-helper/internal/ui/organism/table"
	"github.com/ousiassllc/gsr-helper/internal/ui/token"
)

// Restyle は行・見出し・カーソル・チェックボックス・絞り込みプロンプト・区切り線を新しい配色で
// 描き直す（Issue #28 の受け入れ条件。取り込んだきりにする害は Restyle の doc）。
func TestRestyleAppliesNewPaletteToEveryPart(t *testing.T) {
	dark, light := token.NewStyles(true, true), token.NewStyles(false, true)
	darkWant, lightWant := paletteSamples(dark), paletteSamples(light)

	tbl := table.New(keymap.NewList(), dark, runnerSection(true), orphanSection())
	tbl.SetSize(80, 12)
	tbl.SetItems(0, rows("build01-1", "build01-2"))
	tbl.SetItems(1, rows("old01"))
	// チェックボックスを出し、絞り込みの入力欄も出した状態で見る。
	tbl, _ = send(tbl, "space", "/")

	before := tbl.View()
	for name, want := range darkWant {
		if !strings.Contains(before, want) {
			t.Fatalf("%s が濃色で出ていない（テストの前提が崩れている）:\n%q", name, before)
		}
	}

	tbl.Restyle(keymap.NewList(), light)

	after := tbl.View()
	for name, want := range lightWant {
		if !strings.Contains(after, want) {
			t.Errorf("%s が淡色の配色にならない:\n%q", name, after)
		}
		if strings.Contains(after, darkWant[name]) {
			t.Errorf("%s に濃色向けの配色が残っている:\n%q", name, after)
		}
	}
}

// paletteSamples は一覧の各部が配色から作る文字列を返す。部位ごとに引くのは、行だけ・枠だけが
// 取り残される抜けを 1 本で捕まえるためである（Issue #28 では 4 部位が同時に古かった）。見出しと
// 区切り線は幅いっぱいを埋めるので、中身ではなく装飾の開始列（SGR）で引く（中身を期待値に
// 書くと列幅を変えるたびに壊れる）。
func paletteSamples(s token.Styles) map[string]string {
	return map[string]string{
		"カーソル":     s.Cursor.Render(token.IconCursor),
		"チェックボックス": s.Selected.Render(token.IconChecked),
		"見出し":      sgrPrefix(s.Header),
		"区切り線":     sgrPrefix(s.Divider),
		// 入力中の見出しは bubbles/textinput が Prompt のスタイルで描く。装飾の開始列ではなく
		// 装飾ごと引くのは、区切り線と同じ Muted を使うため開始列だけでは区別できないからである。
		"絞り込みプロンプト": s.Muted.Render(filterPrompt),
	}
}

// sgrPrefix はスタイルが中身の前に置く装飾の開始列を返す。装飾が無ければ空文字になる。
func sgrPrefix(st lipgloss.Style) string {
	prefix, _, _ := strings.Cut(st.Render("x"), "x")
	return prefix
}

// 再スタイルでカーソル位置・選択・絞り込み文字列・フォーカス中の区画は失われない。3 秒ごとの
// 再検出のたびに配色が配られるため、ここで状態が飛ぶと操作の途中でカーソルが先頭へ戻る。
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

// filterPrompt は絞り込みの行の見出し。table パッケージの同名の定数と同じ値を持つ
// （外から参照できないので写している。食い違えば下の検証が前提から落ちる）。
const filterPrompt = "絞り込み: "

// sgrSeq は ANSI の装飾列（SGR）。色が入ったかを列単位で調べるために使う。
var sgrSeq = regexp.MustCompile("\x1b\\[[0-9;]*m")

// 色を無効にした設定（NO_COLOR / --no-color / 非 TTY）では入力欄に色を入れない。
// bubbles/textinput は自分の既定スタイルを持ち、任せると lipgloss のカラープロファイル判定で
// 色が付いて色の可否を決める箇所が 2 つになる（non-functional.md の NO_COLOR を尊重する）。
func TestFilterInputDropsColorWhenColorDisabled(t *testing.T) {
	tbl := table.New(keymap.NewList(), testStyles(), runnerSection(true))
	tbl.SetSize(80, 12)
	tbl.SetItems(0, rows("build01-1"))
	tbl, _ = send(tbl, "/", "b")

	// 前提は View の文字列ではなく状態で見る。既定スタイルは見出しと入力文字の間に ANSI 列を
	// 挟むため、文字列で見ると欠陥のある実装では前提の側が先に落ちて下の検証まで届かない。
	if !tbl.Filtering() || tbl.FilterValue() != "b" {
		t.Fatalf("入力モードになっていない（テストの前提が崩れている）: %v %q", tbl.Filtering(), tbl.FilterValue())
	}

	view := tbl.View()
	for _, seq := range sgrSeq.FindAllString(view, -1) {
		// 反転（\x1b[7m）と、それを閉じる \x1b[m だけは残す。反転は色ではなく入力位置を示す
		// カーソルそのものであり、消すとどこに打っているのか分からなくなる。
		if seq != "\x1b[7m" && seq != "\x1b[m" {
			t.Errorf("色を無効にしても装飾 %q が入る: %q", seq, view)
		}
	}
}
