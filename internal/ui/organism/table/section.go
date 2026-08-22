package table

import (
	btable "charm.land/bubbles/v2/table"
	"charm.land/lipgloss/v2"

	"github.com/ousiassllc/gsr-helper/internal/ui/keymap"
	"github.com/ousiassllc/gsr-helper/internal/ui/molecule"
	"github.com/ousiassllc/gsr-helper/internal/ui/token"
)

const (
	// cursorGutterWidth はカーソル記号（token.IconCursor）を置くガター列の幅。
	cursorGutterWidth = 1
	// checkGutterWidth はチェックボックス（token.IconChecked）を置くガター列の幅。
	checkGutterWidth = 3
	// columnGutter は列と列の間隔。molecule の幅判定（columnPrefix / columnGutter）と
	// 同じ 1 に揃える。ここを広げると molecule が収まると判定した列で桁が溢れる。
	columnGutter = 1
	// headerHeight は bubbles/table が見出しに使う行数。
	headerHeight = 1
)

// section は 1 区画の状態。区画ごとに btable.Model を 1 つ持つ。
type section[T any] struct {
	def   SectionInput[T]
	items []T            // 絞り込み前の全行
	shown []T            // 絞り込み後の行
	cols  []token.Column // 今の幅で表示する列（setWidth が解く）
	tbl   btable.Model
}

// newSection は区画の定義から btable.Model を組み立てる。
func newSection[T any](def SectionInput[T], keys keymap.List, s token.Styles, width int) section[T] {
	sec := section[T]{
		def:   def,
		items: nil,
		shown: nil,
		cols:  nil,
		tbl: btable.New(
			btable.WithKeyMap(tableKeyMap(keys)),
			btable.WithStyles(tableStyles(s)),
		),
	}
	sec.setWidth(width)
	return sec
}

// setWidth は幅に収まる列を解き直して bubbles/table へ渡す。
//
// SectionInput.Columns は「幅が足りる場合に表示する全列」であり、実際に出す列は幅で変わる。
// 解いた結果を区画が持つのは、見出し（bubbles/table が描く）と行（Render が描く）を必ず
// 同じ列から作るためである。
func (s *section[T]) setWidth(w int) {
	// molecule.Columns は新しいスライスを返すので、そのまま書き換えてよい。
	cols := molecule.Columns(s.def.Columns, w, s.def.Rules)
	if n := len(cols); n > 0 {
		// bubbles/table のセルは右に余白を持つ（tableStyles）ので、実際の行幅は最終列の後ろの
		// 余白も含む。molecule の幅判定は列と列の間だけを数えるため、最終列から余白 1 つ分を
		// 引いて突き合わせる。引かないと選択可能な区画で行が 1 セルはみ出す。
		cols[n-1].Width = max(cols[n-1].Width-columnGutter, 0)
	}
	s.cols = cols
	s.tbl.SetColumns(tableColumns(s.def.Selectable, cols))
}

// restyle は区画が bubbles/table へ渡している配色とキー定義を差し替える。
//
// 見出しと選択行の装飾は btable が持つため、Model のフィールドを差し替えるだけでは
// 追随しない。行のセルは Model.refresh が組み立て直す。
func (s *section[T]) restyle(keys keymap.List, st token.Styles) {
	s.tbl.KeyMap = tableKeyMap(keys)
	s.tbl.SetStyles(tableStyles(st))
}

// visible は区画を描くかを返す。行が無い区画は見出しも区切り線も出さない。
func (s section[T]) visible() bool {
	return len(s.shown) > 0
}

// selectable は区画の行を選択できるかを返す。
//
// 識別子を返す関数が無い区画は、選択集合のキーが作れないので選択できない。
func (s section[T]) selectable() bool {
	return s.def.Selectable && s.def.ID != nil
}

// selected はカーソル位置の行を返す。
func (s section[T]) selected() (item T, ok bool) {
	i := s.tbl.Cursor()
	if i < 0 || i >= len(s.shown) {
		var zero T
		return zero, false
	}
	return s.shown[i], true
}

// restoredCursor は行の入れ替えの後にカーソルを置く位置を返す。
//
// 目印の行が残っていればその位置、消えていれば先頭に戻す。添字を据え置くと、
// たまたまその位置に来た別の行を選んだことになる。先頭に戻すのは、利用者が選び直す
// ことが分かる位置であり、破壊的操作の誤爆を避けられるためである。
//
// 目印を取れなかった区画（識別子を返す関数が無い）は今の添字を保つ。
func (s section[T]) restoredCursor(a focusAnchor) int {
	if !a.ok || s.def.ID == nil {
		return s.tbl.Cursor()
	}
	for i, item := range s.shown {
		if s.def.ID(item) == a.id {
			return i
		}
	}
	return 0
}

// render は行のセル列を返す。Render が未設定の区画では空のセルになる。
//
// 選択できない理由も一緒に渡す。理由を落とすと「なぜ選べないのか」を行に出せず、
// 利用者からは反応しない space に見える（screens.md の設計原則 2）。
func (s section[T]) render(item T, styles token.Styles) []string {
	if s.def.Render == nil {
		return nil
	}
	reason, disabled := s.disabledReason(item)
	if !disabled {
		// 選択できる行に理由を渡さない。判定関数は「選べない理由」を常に返す実装に
		// なりがちで（可否だけを行ごとに変える）、そのまま渡すと全行に理由が出る。
		reason = ""
	}
	return s.def.Render(RowInput[T]{
		Item:     item,
		Cols:     s.cols,
		Styles:   styles,
		Reason:   reason,
		Disabled: disabled,
	})
}

// disabledReason は行を選択できないかと、その理由を返す。
// 判定関数が無い区画は全行を選択できる。
func (s section[T]) disabledReason(item T) (reason string, disabled bool) {
	if s.def.Disabled == nil {
		return "", false
	}
	return s.def.Disabled(item)
}

// disabled は行を選択できないかを返す。選択集合の操作は理由を使わない。
func (s section[T]) disabled(item T) bool {
	_, d := s.disabledReason(item)
	return d
}

// dividerHeight は区切り線が使う行数を返す。
func (s section[T]) dividerHeight() int {
	if s.def.Title == "" {
		return 0
	}
	return 1
}

// chromeHeight は区画の行以外が使う行数（区切り線 + 見出し）を返す。
func (s section[T]) chromeHeight() int {
	return s.dividerHeight() + headerHeight
}

// tableColumns は区画の列定義を bubbles/table の列へ変換する。
//
// 先頭にカーソル用、選択可能な区画にはチェックボックス用のガター列を足す。ガター列を
// 列定義側にも足すのは、セル数と列数を一致させるためである（fitCells を参照）。
func tableColumns(selectable bool, cols []token.Column) []btable.Column {
	out := make([]btable.Column, 0, len(cols)+2)
	out = append(out, btable.Column{Title: "", Width: cursorGutterWidth})
	if selectable {
		out = append(out, btable.Column{Title: "", Width: checkGutterWidth})
	}
	for _, c := range cols {
		out = append(out, btable.Column{Title: c.Title, Width: c.Width})
	}
	return out
}

// tableStyles は bubbles/table のスタイルを組み立てる。
//
// 選択行に色を付けないのは、セルが molecule の段階で装飾済みであり、行全体へ前景色を
// 重ねると ANSI 列が入れ子になって崩れるためである。カーソル位置はガター列の記号
// （token.IconCursor）で示すので、色に頼らずに判別できる。
func tableStyles(s token.Styles) btable.Styles {
	return btable.Styles{
		Header:   s.Header.PaddingRight(columnGutter),
		Cell:     lipgloss.NewStyle().PaddingRight(columnGutter),
		Selected: lipgloss.NewStyle(),
	}
}

// fitCells はセル数を列数に揃える。
//
// bubbles/table は行のセルを走査しながら同じ添字の列定義を引くため、セル数が列数を
// 超えると添字範囲外で panic する。RenderRow は呼び出し側の関数であり列数と違う数を
// 返す実装を防げないので、ここで必ず揃える。
func fitCells(cells []string, n int) []string {
	if n <= 0 {
		return nil
	}
	if len(cells) == n {
		return cells
	}
	if len(cells) > n {
		return cells[:n]
	}

	out := make([]string, n)
	copy(out, cells)
	return out
}
