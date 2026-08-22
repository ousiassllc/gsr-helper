package table_test

import (
	"slices"
	"strings"
	"testing"

	"charm.land/bubbles/v2/textinput"
	"charm.land/lipgloss/v2"

	"github.com/ousiassllc/gsr-helper/internal/ui/keymap"
	"github.com/ousiassllc/gsr-helper/internal/ui/organism/table"
	"github.com/ousiassllc/gsr-helper/internal/ui/token"
)

// 一覧のカーソルは screens.md のキーマップどおりに動き、bubbles/table の既定キー
// （u / d / f / b / space など）ではスクロールしない（理由は tableKeyMap を参照）。
func TestTableCursorKeys(t *testing.T) {
	items := rows("build01-1", "build01-2", "build01-3", "build01-4", "build01-5")

	tests := map[string]struct {
		keys []string
		want string
	}{
		"初期位置は先頭":       {nil, "build01-1"},
		"j で下へ":         {[]string{"j"}, "build01-2"},
		"↓ で下へ":         {[]string{"down"}, "build01-2"},
		"k で上へ":         {[]string{"j", "j", "k"}, "build01-2"},
		"↑ で上へ":         {[]string{"j", "up"}, "build01-1"},
		"先頭より上へは動かない":   {[]string{"k", "k"}, "build01-1"},
		"G で末尾へ":        {[]string{"G"}, "build01-5"},
		"g で先頭へ":        {[]string{"G", "g"}, "build01-1"},
		"末尾より下へは動かない":   {[]string{"G", "j"}, "build01-5"},
		"ctrl+f でページ送り": {[]string{"ctrl+f"}, "build01-5"},
		"ctrl+b で前のページ": {[]string{"ctrl+f", "ctrl+b"}, "build01-1"},
	}
	for _, k := range []string{"u", "d", "f", "b", "space", "pgdown", "pgup", "home", "end"} {
		tests["既定キー "+k+" では動かない"] = struct {
			keys []string
			want string
		}{[]string{"j", k}, "build01-2"}
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			tbl, _ := send(newTable(true, items), tt.keys...)
			if got := selectedName(tbl); got != tt.want {
				t.Errorf("カーソル位置 = %q, want %q", got, tt.want)
			}
		})
	}
}

// space で選択が切り替わり、ctrl+a で全選択される。チェックボックスは 1 件も選択されて
// いない間は出さない（screens.md の記号表は「選択モード時のみ表示」）。桁は動かない。
func TestTableSelection(t *testing.T) {
	all := []string{"build01-1", "build01-2", "build01-3"}
	tbl := newTable(true, rows(all...))
	empty := tbl.View()
	if strings.Contains(empty, token.IconUnchecked) {
		t.Error("1 件も選択していないのにチェックボックスが出ている")
	}

	for _, tt := range []struct {
		keys []string
		want []string
	}{
		{[]string{"space"}, all[:1]},
		{[]string{"space"}, []string{}},
		{[]string{"j", "space", "ctrl+a"}, all},
	} {
		tbl, _ = send(tbl, tt.keys...)
		if got := names(tbl.Checked()); !slices.Equal(got, tt.want) {
			t.Errorf("%v の後の選択 = %v, want %v", tt.keys, got, tt.want)
		}
	}

	selected := tbl.View()
	if !strings.Contains(selected, token.IconChecked) {
		t.Error("選択後にチェックボックスが出ていない")
	}
	if got, want := lipgloss.Width(selected), lipgloss.Width(empty); got != want {
		t.Errorf("チェックボックスの有無で幅が動いた（%d → %d）", want, got)
	}

	tbl.ClearSelection()
	if got := names(tbl.Checked()); len(got) != 0 {
		t.Errorf("ClearSelection の後の選択 = %v, want なし", got)
	}
}

// 選択できない行では space / ctrl+a を無視する。区画ごとの可否では表せない行ごとの可否
// （Disk タブのジョブ実行中の _work）を SectionInput.Disabled で表す。
func TestTableIgnoresSelectionOnDisabledRows(t *testing.T) {
	sec := runnerSection(true)
	sec.Disabled = func(r row) (string, bool) { return "ジョブ実行中です", r.name == "build01-2" }
	tbl := table.New(keymap.NewList(), testStyles(), sec)
	tbl.SetSize(80, 12)
	tbl.SetItems(0, rows("build01-1", "build01-2", "build01-3"))

	tbl, _ = send(tbl, "j", "space")
	if got := names(tbl.Checked()); len(got) != 0 {
		t.Errorf("選択できない行が space で選択された（%v）", got)
	}

	tbl, _ = send(tbl, "ctrl+a")
	if got := names(tbl.Checked()); !slices.Equal(got, []string{"build01-1", "build01-3"}) {
		t.Errorf("ctrl+a の後の選択 = %v, want [build01-1 build01-3]", got)
	}
}

// 入力モード中はグローバルキーも一覧のキーも解釈せず、打った文字が入力欄に入る。
func TestTableFilterInputModeSwallowsGlobalKeys(t *testing.T) {
	tbl, cmd := send(newTable(true, rows("build01-1", "build01-2", "build01-3")), "space", "/")
	if !tbl.Filtering() {
		t.Fatal("/ で入力モードに入っていない")
	}
	if cmd == nil {
		t.Error("textinput.Focus の Cmd が捨てられている（カーソルが点滅しない）")
	}

	// 1〜7 / r / ? / q / j / G はすべて入力文字として扱う（tab は文字を持たない）。
	tbl, _ = send(tbl, "1", "r", "?", "q", "j", "G", "tab")
	if got := tbl.FilterValue(); got != "1r?qjG" {
		t.Errorf("入力欄 = %q, want %q", got, "1r?qjG")
	}
	if got := names(tbl.Checked()); !slices.Equal(got, []string{"build01-1"}) {
		t.Errorf("入力中に選択が変わった（%v）", got)
	}

	// 入力中に打った j / G が一覧のカーソルを動かしていないことを、取消後に確かめる。
	tbl, _ = send(tbl, "esc")
	if got := selectedName(tbl); got != "build01-1" {
		t.Errorf("入力中にカーソルが動いた（%q）", got)
	}
}

// enter で絞り込みを確定し、esc で取り消す。確定済みの絞り込みは page が解除できる。
func TestTableFilterAcceptAndCancel(t *testing.T) {
	all := []string{"build01-1", "build01-2", "web-1"}
	tbl, _ := send(newTable(true, rows(all...)), "/", "w", "e", "b")
	if got := names(tbl.Shown(0)); !slices.Equal(got, []string{"web-1"}) {
		t.Errorf("入力中の表示 = %v, want [web-1]", got)
	}

	tbl, _ = send(tbl, "enter")
	if tbl.Filtering() || tbl.FilterValue() != "web" || selectedName(tbl) != "web-1" {
		t.Errorf("確定後: 入力中 = %v, 絞り込み = %q, カーソル = %q（want false / web / web-1）",
			tbl.Filtering(), tbl.FilterValue(), selectedName(tbl))
	}

	tbl.ClearFilter()
	if got := names(tbl.Shown(0)); tbl.FilterValue() != "" || !slices.Equal(got, all) {
		t.Errorf("ClearFilter 後: 絞り込み = %q, 表示 = %v", tbl.FilterValue(), got)
	}

	// 入力中の esc は取消。再編集して取り消したときは編集前の値へ戻さず全消去する。
	tbl, _ = send(tbl, "/", "w", "enter", "/", "e", "esc")
	if tbl.Filtering() || tbl.FilterValue() != "" {
		t.Errorf("取消後: 入力中 = %v, 絞り込み = %q（want false / 空）", tbl.Filtering(), tbl.FilterValue())
	}
	if got := names(tbl.Shown(0)); !slices.Equal(got, all) {
		t.Errorf("取消後の表示 = %v, want %v", got, all)
	}
	if got := tbl.Shown(9); got != nil {
		t.Errorf("無い区画の表示 = %v, want nil", got)
	}
}

// SetItems の後もカーソルは行の範囲に収まり、消えた行の選択は残らない（3 秒ごとの
// 再検出で検出漏れした runner が戻ったときに、消えていた選択が復活しない）。
//
// カーソルを置いていた行そのものが消えた場合は先頭へ戻す（restoredCursor の doc）。
// 添字を切り詰めて据えると、たまたまその位置に来た別の runner を選んだことになる。
func TestTableSetItems(t *testing.T) {
	tbl, _ := send(newTable(true, rows("a-1", "a-2", "a-3", "a-4", "a-5")), "G", "ctrl+a")

	tbl.SetItems(0, rows("a-1", "a-2"))
	if got := selectedName(tbl); got != "a-1" {
		t.Errorf("カーソル位置 = %q, want a-1（選択していた行が消えたら先頭へ戻す）", got)
	}
	if got := names(tbl.Checked()); !slices.Equal(got, []string{"a-1", "a-2"}) {
		t.Errorf("消えた行を含む選択 = %v, want [a-1 a-2]", got)
	}

	tbl.SetItems(0, rows("a-1", "a-2", "a-3"))
	if got := names(tbl.Checked()); !slices.Equal(got, []string{"a-1", "a-2"}) {
		t.Errorf("再出現後の選択 = %v, want [a-1 a-2]（消えていた選択は復活しない）", got)
	}

	tbl.SetItems(0, nil)
	if _, ok := tbl.Selected(); ok {
		t.Error("行が無いのにカーソル位置が取れている")
	}
	if got := tbl.View(); got != "" {
		t.Errorf("行が無いときの表示 = %q, want 空（文言は page が出す）", got)
	}
}

// SetItems に渡したスライスと Shown が返したスライスは、どちらも Table の内部と
// 共有しない。page が手元のスライスを使い回して書き換えたときに、表示中の行や
// 選択の突き合わせが崩れないようにするためである。
func TestTableDoesNotShareItemSlices(t *testing.T) {
	want := []string{"a-1", "a-2", "a-3"}
	items := rows(want...)
	tbl, _ := send(newTable(true, items), "ctrl+a")

	// 渡したスライスを書き換えても内部は変わらない。
	items[0] = row{name: "書き換え", note: ""}
	if got := names(tbl.Shown(0)); !slices.Equal(got, want) {
		t.Errorf("渡したスライスの書き換え後の表示 = %v, want %v", got, want)
	}
	if got := names(tbl.Checked()); !slices.Equal(got, want) {
		t.Errorf("渡したスライスの書き換え後の選択 = %v, want %v", got, want)
	}

	// 返ったスライスを書き換えても内部は変わらない。
	shown := tbl.Shown(0)
	shown[0] = row{name: "書き換え", note: ""}
	if got := names(tbl.Shown(0)); !slices.Equal(got, want) {
		t.Errorf("返ったスライスの書き換え後の表示 = %v, want %v", got, want)
	}
}

// キー以外の Msg は入力欄へ流す。カーソルの点滅は Msg と Cmd の往復で続くため、
// ここで Cmd を捨てると入力中にカーソルが出なくなる。
func TestTableForwardsNonKeyMessagesToFilter(t *testing.T) {
	tbl, _ := send(newTable(true, rows("build01-1", "build01-2")), "/")

	tbl, cmd := tbl.Update(textinput.Blink())
	if cmd == nil {
		t.Error("点滅の Msg に対する Cmd が捨てられている")
	}
	if !tbl.Filtering() || selectedName(tbl) != "build01-1" {
		t.Error("キー以外の Msg で状態が変わった")
	}
}

// 範囲外の区画や描画関数を持たない区画でも panic しない。
func TestTableWithIncompleteSectionInput(t *testing.T) {
	sec := runnerSection(false)
	sec.Render = nil
	sec.ID = nil

	tbl := table.New(keymap.NewList(), testStyles(), sec)
	tbl.SetSize(80, 12)
	tbl.SetItems(0, rows("build01-1"))
	tbl.SetItems(-1, rows("無い区画"))
	tbl.SetItems(9, rows("無い区画"))

	tbl, _ = send(tbl, "j", "space", "ctrl+a")
	if got := len(tbl.Checked()); got != 0 {
		t.Errorf("識別子を返せない区画の選択件数 = %d, want 0", got)
	}
	if _, ok := tbl.Selected(); !ok {
		t.Error("行があるのにカーソル位置が取れない")
	}
}
