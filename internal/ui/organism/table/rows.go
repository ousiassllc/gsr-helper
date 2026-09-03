package table

import (
	btable "charm.land/bubbles/v2/table"
	"charm.land/lipgloss/v2"

	"github.com/ousiassllc/gsr-helper/internal/ui/atom"
)

// 選択集合と絞り込みの操作、および行データの組み立てを集める。

// toggleChecked はカーソル位置の行の選択を切り替える。
//
// 選択できない区画と行ではキーを無視する（行ごとの可否は SectionInput.Disabled）。
func (t *Model[T]) toggleChecked() {
	cur, ok := t.current()
	if !ok || !cur.selectable() {
		return
	}
	item, ok := cur.selected()
	if !ok || cur.disabled(item) {
		return
	}

	id := cur.def.ID(item)
	if t.checked[id] {
		delete(t.checked, id)
	} else {
		t.checked[id] = true
	}
	t.refresh(t.focus)
}

// checkAll は選択可能な区画の、絞り込み後に見えている行をすべて選択する。
//
// 絞り込み後の行に限るのは、画面に出ていない行まで選択すると、続く確認ダイアログで
// 初めて対象を知ることになるためである。
func (t *Model[T]) checkAll() {
	for i := range t.sections {
		s := &t.sections[i]
		if !s.selectable() {
			continue
		}
		for _, item := range s.shown {
			if s.disabled(item) {
				continue
			}
			t.checked[s.def.ID(item)] = true
		}
		t.refresh(i)
	}
}

// pruneChecked は現存しない行の選択を捨てる。
//
// 選択集合のキーは行の識別子だけなので、行が消えても選択は残る。残すと同じ識別子の
// 行が再び現れたときに選択が復活し、map も単調に増え続ける。
func (t *Model[T]) pruneChecked() {
	if len(t.checked) == 0 {
		return
	}

	live := make(map[string]bool, len(t.checked))
	for i := range t.sections {
		s := &t.sections[i]
		if !s.selectable() {
			continue
		}
		for _, item := range s.items {
			if id := s.def.ID(item); t.checked[id] {
				live[id] = true
			}
		}
	}
	if len(live) == len(t.checked) {
		return
	}
	// 選択が 0 件になるとチェックボックスは全区画で消える（checkState を参照）。
	t.checked = live
	t.refreshAll()
}

// stopFiltering は入力モードを終える。cancel が真なら絞り込みを取り消す。
func (t *Model[T]) stopFiltering(cancel bool) {
	t.filtering = false
	t.filter.Blur()
	if cancel {
		// 取消は編集前の値へ戻さず全消去する（解除の規則を 1 つにまとめる）。
		t.filter.Reset()
	}
	t.applyFilter()
}

// filterVisible は絞り込みの行を出すかを返す。
func (t Model[T]) filterVisible() bool {
	return t.filtering || t.filter.Value() != ""
}

// filterView は絞り込みの行を返す。入力中は入力欄、確定後は絞り込み中である旨を出す。
//
// 出力は一覧の幅で切る。絞り込み文字列の長さは利用者が決めるため、切らないと領域を
// 超えて枠が崩れる。template.Frame の切り詰めは最後の防波堤であり、幅を持っている
// organism が自分の領域に収めるのが筋である。atom.Truncate ではなく lipgloss を使うのは、
// 入力欄の出力がカーソルの装飾（ANSI 列）を含み、装飾済みの文字列は atom.Truncate の
// 切り詰めで壊れるためである（中略記号を足さないのも入力中の 1 桁を惜しむため）。
func (t Model[T]) filterView() string {
	var out string
	if t.filtering {
		out = t.filter.View()
	} else {
		out = t.styles.Muted.Render(filterPrompt + t.filter.Value())
	}
	// 幅が 0 以下（SetSize より前）の場合に 1 桁へ丸めるのは template.Frame と同じ扱い。
	return lipgloss.NewStyle().MaxWidth(max(t.width, 1)).Render(out)
}

// applyFilter は絞り込み文字列を全区画に反映する。
func (t *Model[T]) applyFilter() {
	// 目印は絞り込む前に取る。絞り込みで上の行が落ちても、残っていれば同じ行に留まる。
	a := t.anchor()
	for i := range t.sections {
		t.filterSection(i)
	}
	t.normalizeFocus(a)
	t.layout()
}

// filterSection は 1 区画の表示対象を絞り込む。
//
// 一致判定を持たない区画は絞り込まない。孤児ユニットのように絞り込みの対象に
// しない区画を、判定関数を渡さないことで表せる。
func (t *Model[T]) filterSection(i int) {
	s := &t.sections[i]
	q := t.filter.Value()
	if q == "" || s.def.Match == nil {
		s.shown = s.items
		t.refresh(i)
		return
	}

	shown := make([]T, 0, len(s.items))
	for _, item := range s.items {
		if s.def.Match(item, q) {
			shown = append(shown, item)
		}
	}
	s.shown = shown
	t.refresh(i)
}

// refreshAll は全区画の行を組み立て直す。
//
// 選択集合が変わるとチェックボックスの表示は全区画で変わる（checkState を参照）。
func (t *Model[T]) refreshAll() {
	for i := range t.sections {
		t.refresh(i)
	}
}

// refresh は行データを組み立て直して bubbles/table へ渡す。
//
// カーソル記号（token.IconCursor）は bubbles/table が描かないため、ガター列として行データに
// 埋め込む。カーソルを色に頼らず記号で示すためである（screens.md の設計原則 4）。
//
// 行が 0 件になると bubbles/table のカーソルは -1 になり、行が戻っても -1 のままで、その
// table は 1 行も描かない。フォーカスの無い区画（孤児ユニットなど）は normalizeFocus が
// 触らないため、ここで先頭へ戻すしかない。
func (t *Model[T]) refresh(i int) {
	s := &t.sections[i]
	cursor := min(max(s.tbl.Cursor(), 0), len(s.shown)-1)
	rows := make([]btable.Row, 0, len(s.shown))
	for j, item := range s.shown {
		rows = append(rows, t.row(s, item, t.focus == i && j == cursor))
	}
	s.tbl.SetRows(rows)
	if len(rows) > 0 && s.tbl.Cursor() < 0 {
		s.tbl.SetCursor(0)
	}
}

// row は 1 行を bubbles/table の行へ変換する。
func (t Model[T]) row(s *section[T], item T, onCursor bool) btable.Row {
	cells := make([]string, 0, len(s.cols)+2)
	cells = append(cells, atom.Cursor(onCursor, t.styles))
	if s.def.Selectable {
		cells = append(cells, atom.Checkbox(t.checkState(s, item), t.styles))
	}
	return append(cells, fitCells(s.render(item, t.styles), len(s.cols))...)
}

// checkState は行のチェックボックスの表示状態を返す。
//
// 1 件も選択されていない間と選択できない行では表示しない（screens.md の記号表は
// 「選択モード時のみ表示」と定める）。非表示でも記号と同じ幅の空白なので桁は動かない。
func (t Model[T]) checkState(s *section[T], item T) atom.CheckState {
	if !s.selectable() || len(t.checked) == 0 || s.disabled(item) {
		return atom.CheckHidden
	}
	if t.checked[s.def.ID(item)] {
		return atom.CheckOn
	}
	return atom.CheckOff
}
