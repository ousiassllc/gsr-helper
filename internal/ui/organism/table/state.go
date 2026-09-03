package table

// current はフォーカス中の区画を返す。
func (t Model[T]) current() (*section[T], bool) {
	if t.focus < 0 || t.focus >= len(t.sections) {
		return nil, false
	}
	return &t.sections[t.focus], true
}

// visibleSections は行を持つ区画の添字を並び順に返す。
func (t Model[T]) visibleSections() []int {
	out := make([]int, 0, len(t.sections))
	for i := range t.sections {
		if t.sections[i].visible() {
			out = append(out, i)
		}
	}
	return out
}

// nextVisible は from から step の向きで最初に見つかる、行を持つ区画を返す。
func (t Model[T]) nextVisible(from, step int) (int, bool) {
	for i := from + step; i >= 0 && i < len(t.sections); i += step {
		if t.sections[i].visible() {
			return i, true
		}
	}
	return 0, false
}

// crossSection は区画の端でカーソルを次（前）の区画へ移す。移したかを返す。
//
// 区画をまたぐ移動を Table が持つのは、bubbles/table が自分の行の範囲でしかカーソルを
// 動かさないためである。移動先では安全側の端（下へ移るときは先頭）にカーソルを置く。
func (t *Model[T]) crossSection(step int) bool {
	cur, ok := t.current()
	if !ok {
		return false
	}

	atEdge := cur.tbl.Cursor() <= 0
	if step > 0 {
		atEdge = cur.tbl.Cursor() >= len(cur.shown)-1
	}
	if !atEdge {
		return false
	}

	next, ok := t.nextVisible(t.focus, step)
	if !ok {
		return false
	}

	cursor := 0
	if step < 0 {
		cursor = len(t.sections[next].shown) - 1
	}
	t.setFocus(next, cursor)
	return true
}

// gotoEdge は一覧全体の先頭（step < 0）または末尾（step > 0）へカーソルを移す。
//
// 区画の中に留めないのは、j / k が区画をまたぐのに G がまたがないと、利用者から見て
// 「末尾」の意味が 2 つになるためである。
func (t *Model[T]) gotoEdge(step int) {
	visible := t.visibleSections()
	if len(visible) == 0 {
		return
	}
	if step < 0 {
		t.setFocus(visible[0], 0)
		return
	}

	last := visible[len(visible)-1]
	t.setFocus(last, len(t.sections[last].shown)-1)
}

// setFocus はキー入力を受け取る区画とカーソル位置を設定する。
func (t *Model[T]) setFocus(i, cursor int) {
	if i < 0 || i >= len(t.sections) {
		return
	}

	prev := t.focus
	t.focus = i
	for j := range t.sections {
		if j == i {
			t.sections[j].tbl.Focus()
			continue
		}
		t.sections[j].tbl.Blur()
	}
	t.setCursor(i, cursor)
	if prev != i && prev >= 0 && prev < len(t.sections) {
		// 前の区画のカーソル記号を消す。
		t.refresh(prev)
	}
}

// setCursor は区画のカーソル位置を設定し、カーソル記号を行へ追随させる。
func (t *Model[T]) setCursor(i, cursor int) {
	t.sections[i].tbl.SetCursor(max(cursor, 0))
	t.refresh(i)
}

// setSectionWidth は区画の列を幅から解き直し、行とカーソル位置を作り直す。
//
// 列を差し替える前に行を空にするのは、bubbles/table が列の差し替えで手元の行を描き直す
// ため、セル数が新しい列数を超えると添字範囲外で panic するからである。空にするとカーソルが
// -1 に落ちるので控えた位置へ戻す。
func (t *Model[T]) setSectionWidth(i, w int) {
	s := &t.sections[i]
	cursor := s.tbl.Cursor()
	s.tbl.SetRows(nil)
	s.setWidth(w)
	t.refresh(i)
	t.setCursor(i, cursor)
}

// focusAnchor は行の入れ替えをまたいでカーソルを貼り直すための目印。
//
// 識別子を返す関数を持たない区画では ok が false になり、貼り直しは添字に退避する。
type focusAnchor struct {
	id string
	ok bool
}

// anchor は今カーソルがある行の目印を取る。行を入れ替える前に呼ぶこと。
func (t Model[T]) anchor() focusAnchor {
	cur, ok := t.current()
	if !ok || cur.def.ID == nil {
		return focusAnchor{id: "", ok: false}
	}
	item, ok := cur.selected()
	if !ok {
		return focusAnchor{id: "", ok: false}
	}
	return focusAnchor{id: cur.def.ID(item), ok: true}
}

// normalizeFocus は行の入れ替えの後にカーソルを貼り直し、行が無くなった区画からは
// フォーカスを外す。
//
// 3 秒ごとの再検出で行が入れ替わるため、フォーカスを毎回先頭へ戻さず、
// 今の区画に行が残っている限りはそこに留める。
//
// **カーソルは添字ではなく識別子で貼り直す。** カーソルより上の行が消えると並びが
// 詰まり、添字を当て直したのでは別の行が選ばれる。Selected() の結果はフッタの操作
// 可否・enter の詳細・サービス制御の対象になるため、ずれは「選んだつもりとは別の
// runner を操作する」に化ける（runnerdetail.Model.SetState が Dir で引き直すのと
// 同じ危険である）。
func (t *Model[T]) normalizeFocus(a focusAnchor) {
	if cur, ok := t.current(); ok && cur.visible() {
		t.setFocus(t.focus, cur.restoredCursor(a))
		return
	}
	for i := range t.sections {
		if t.sections[i].visible() {
			t.setFocus(i, 0)
			return
		}
	}
	t.setFocus(0, 0)
}

// layout は本体の高さを区画へ配る。
//
// 先の区画を優先し、後続の区画には最低 1 行を残す。区画は「一覧 + 孤児ユニット」のように
// 主と補助の組で使うため、主の区画へ行を多く配る方が読みやすい。最低行数の合計が高さを
// 超える場合は配り切れないが、下限を優先する（高さに収める切り詰めは View が行う）。
func (t *Model[T]) layout() {
	visible := t.visibleSections()
	avail := t.height
	if t.filterVisible() {
		// 絞り込みの行に 1 行使う。
		avail--
	}

	for idx, i := range visible {
		reserve := 0
		for _, j := range visible[idx+1:] {
			reserve += t.sections[j].chromeHeight() + 1
		}

		s := &t.sections[i]
		need := s.chromeHeight() + len(s.shown)
		alloc := max(min(need, avail-reserve), s.chromeHeight()+1)
		s.tbl.SetWidth(t.width)
		// SetHeight は内部で見出しの高さを引くため、区切り線の分だけを引いて渡す。
		s.tbl.SetHeight(alloc - s.dividerHeight())
		avail -= alloc
	}
}
