package table

import "slices"

// page が呼ぶ入口（状態の差し替えと問い合わせ）をここに集める。

// SetItems は区画の行を差し替える。
//
// 差し替えで消えた行の選択は捨てる。3 秒ごとの再検出で一時的に検出漏れした runner が
// 戻ったときに、画面から消えていた選択が復活しないようにするためである。
//
// 渡されたスライスは写しを取って持つ。呼び出し側のスライスをそのまま指すと、page が
// 手元のスライスを使い回して書き換えたときに一覧の状態が崩れる。行数は高々数十なので、
// 確保の費用より状態が壊れる事故の重さを採る（Shown も同じ理由で写しを返す）。
func (t *Model[T]) SetItems(section int, items []T) {
	if section < 0 || section >= len(t.sections) {
		return
	}
	t.sections[section].items = slices.Clone(items)
	t.pruneChecked()
	t.filterSection(section)
	t.normalizeFocus()
	t.layout()
}

// SetSize は一覧に割り当てられた領域を設定し、表示する列を幅から解き直す。
//
// 列の集合は幅に依存する（molecule.Columns）ためリサイズのたびに変わるが、Table を
// 作り直すとカーソル位置・選択集合・絞り込みが失われる。
//
// 大きさが変わっていなければ何もしない。共有状態は自動更新のたびに全 page へ配られる
// が、そのほとんどでサイズは変わらず、毎回解き直すと 1 周期で全タブ・全区画ぶんの
// 全行再構築が積み上がるためである。
func (t *Model[T]) SetSize(w, h int) {
	if t.width == w && t.height == h {
		return
	}
	t.width, t.height = w, h
	for i := range t.sections {
		t.setSectionWidth(i, w)
	}
	t.layout()
}

// Selected はカーソル位置の行を返す。
func (t Model[T]) Selected() (item T, ok bool) {
	cur, ok := t.current()
	if !ok {
		var zero T
		return zero, false
	}
	return cur.selected()
}

// Checked は選択済みの行を区画の順に返す。
//
// 絞り込みで見えていない行も含める。絞り込みを取り消したときに選択が消えていると、
// 選んだ対象を数え直すことになるためである。
func (t Model[T]) Checked() []T {
	out := make([]T, 0, len(t.checked))
	for i := range t.sections {
		s := &t.sections[i]
		if !s.selectable() {
			continue
		}
		for _, item := range s.items {
			if t.checked[s.def.ID(item)] {
				out = append(out, item)
			}
		}
	}
	return out
}

// Shown は絞り込み後に表示している行を返す。page は状態行の「N 件」に使う。
// 行そのものを返すので、絞り込みの結果を View() の文字列一致で確かめずに済む。
//
// 内部のスライスをそのまま返さず写しを返す。返り値を呼び出し側が書き換えると Table の
// 表示が崩れるためである。View のたびに確保することになるが、行数は高々数十であり、
// 状態が壊れる事故の重さと比べれば許容できる。
func (t Model[T]) Shown(section int) []T {
	if section < 0 || section >= len(t.sections) {
		return nil
	}
	return slices.Clone(t.sections[section].shown)
}

// FocusedSection はカーソルがある区画の添字を返す。Runners タブは孤児ユニットの行と
// runner の行で有効なキーもフッタの文言も変わる。
func (t Model[T]) FocusedSection() int { return t.focus }

// Filtering は入力モードかを返す。page は状態行に「入力中」を出すために使う。
func (t Model[T]) Filtering() bool { return t.filtering }

// FilterValue は絞り込み文字列を返す。
func (t Model[T]) FilterValue() string { return t.filter.Value() }

// ClearSelection は選択を解除する（esc の「選択のクリア」）。
func (t *Model[T]) ClearSelection() {
	t.checked = make(map[string]bool)
	t.refreshAll()
}

// ClearFilter は確定済みの絞り込みを解除する（esc の「1 つ前の状態へ戻る」）。
// 入力中の esc は Update が取消として処理するため、確定後の経路が別に必要になる。
func (t *Model[T]) ClearFilter() {
	t.filter.Reset()
	t.applyFilter()
}
