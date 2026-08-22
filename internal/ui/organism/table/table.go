// Package table は区画（セクション）に分かれた一覧の共通実装を提供する。
//
// どのタブの一覧も「カーソル・複数選択・絞り込み・区画をまたぐ移動」という同じ
// ローカル状態を持つため、organism 本体から独立した 1 パッケージに切り出してある。
// organism（ChoiceList）・organism/pane（Detail / Help）とは互いに import せず、
// 必要なものを選んで組み合わせるのは page の役割である。
//
// bubbles/table は btable として import する。パッケージ名がこのパッケージ自身と
// 衝突するためであり、ラップしている相手が bubbles であることを読み手に示す意図もある。
//
// この階層の型は tea.Model を実装せず、bubbles 流の「具体型を返す Update と
// View() string」に揃える。ジェネリックな Model[T] の Update の戻りを tea.Model に潰すと
// 呼び出し側で毎回型アサーションが必要になって panic の経路が増え、View() を tea.View に
// すると organism を縦に並べるたびに文字列へ戻す処理が入るためである（「interface は
// Executor / doctor.Check / tea.Model の 3 つに限る」規約は page と親 Model が満たす）。
// スクロール・計時・テキスト入力は自前で実装せず bubbles に委ねる。
package table

import (
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/ui/atom"
	"github.com/ousiassllc/gsr-helper/internal/ui/keymap"
	"github.com/ousiassllc/gsr-helper/internal/ui/token"
)

// filterPrompt は絞り込みの行の見出し。入力中と確定後で同じ文字列を使う。
const filterPrompt = "絞り込み: "

// RowInput は 1 行を描くのに必要な値をまとめたもの。
//
// 引数を構造体にするのは、行の描画に渡す値が増えても Render の署名が変わらないように
// するためである。Render はタブごとに差し替えて使い回すので、署名を変えると既存の
// 全タブと organism のテストに波及する（選択不可の理由を後から足せなかったのがこの形に
// した理由である）。
//
// チェックボックスとカーソル記号のセルは渡さない。行頭のガター列を描くのは Model 自身で
// あり、Render は Cols と同じ順・同じ数のセルだけを返す。
type RowInput[T any] struct {
	Item   T
	Cols   []token.Column // この幅で実際に表示する列。セルはこの数だけ返すこと
	Styles token.Styles
	// Reason は行を選択できない理由。Disabled が真のときだけ入る。
	//
	// 行末のセルに載せるかは Render が決める。理由を置く列を持っているのは
	// Render 側（token.Column の集合を決めるのはタブ）だからである。
	Reason string
	// Disabled は行を選択できないか。SectionInput.Disabled の判定結果であり、
	// Render はチェックボックスを描かないが、行全体を薄く描くなどの判断に使える。
	Disabled bool
}

// RenderRow は 1 行をセルの列に変換する。molecule の *Row 関数（RunnerRow / JobRow /
// OrphanRow）を RowInput から呼ぶ薄い関数を page 側に置いて渡す。
type RenderRow[T any] func(in RowInput[T]) []string

// RowID は行の識別子を返す。選択集合のキーに使う。
type RowID[T any] func(item T) string

// RowDisabled は行を選択できないかと、その理由を返す。区画ごとの可否（Selectable）では
// 表せない行ごとの可否を表す。Disk タブはジョブ実行中の _work だけを選択不可にするため、
// 1 つの区画に選択可・不可が混在する。
//
// 返した理由は RowInput.Reason として Render へ渡る。判定と表示を分けているのは、理由を
// どの列に置くかがタブごとに違う（列の集合を決めるのはタブ）ためである。
type RowDisabled[T any] func(item T) (reason string, disabled bool)

// SectionInput は 1 区画の定義。
//
// 区画は Runners タブの孤児ユニットのように、1 つの一覧の中で別扱いにする
// 行の集まりを表す。
type SectionInput[T any] struct {
	Title      string                      // 区切り線の見出し。空なら区切り線を出さない
	Columns    []token.Column              // 幅が足りる場合に表示する全列。実際に出す列は SetSize が解く
	Rules      token.ColumnRules           // 幅が足りないときの落とし方。ゼロ値なら末尾から落とす
	Render     RenderRow[T]                // 1 行をセルの列に変換する関数
	ID         RowID[T]                    // 行の識別子を返す関数
	Match      func(item T, q string) bool // 絞り込みの一致判定
	Disabled   RowDisabled[T]              // 行ごとの選択可否。nil なら全行を選択できる
	Selectable bool                        // 区画ごとの選択可否
}

// Model は一覧の共通実装。bubbles/table のラッパーであり、区画（セクション）ごとに
// btable.Model を 1 つ持つ。
//
// カーソル移動・スクロール・列幅の調整は bubbles/table に委ね、区画の並べ方・
// 区画をまたぐカーソル移動・複数選択・絞り込みを受け持つ。
//
// **コピーは状態を共有する（値としての独立性はない）。** 区画のスライスと選択集合の map は
// 写しても同じ実体を指すため、写した側でカーソルを動かすと元の値も動く。bubbles 流の署名に
// 揃えた結果であり、page は直前の Update が返した 1 つの値だけを持つこと。
type Model[T any] struct {
	sections  []section[T]
	focus     int             // キー入力を受け取る区画
	checked   map[string]bool // 選択集合。キーは行の識別子
	filter    textinput.Model // 絞り込み
	filtering bool            // 入力モードか
	keys      keymap.List
	styles    token.Styles
	width     int
	height    int
}

// New は区画の定義から一覧を組み立てる。
func New[T any](keys keymap.List, s token.Styles, secs ...SectionInput[T]) Model[T] {
	in := textinput.New()
	in.Prompt = filterPrompt
	in.SetStyles(filterStyles(s))

	t := Model[T]{
		sections:  make([]section[T], 0, len(secs)),
		focus:     0,
		checked:   make(map[string]bool),
		filter:    in,
		filtering: false,
		keys:      keys,
		styles:    s,
		width:     0,
		height:    0,
	}
	for _, def := range secs {
		t.sections = append(t.sections, newSection(def, keys, s, 0))
	}
	t.setFocus(0, 0)
	return t
}

// filterStyles は token のスタイルを bubbles/textinput のスタイルへ写す。
//
// 既定スタイル（textinput.DefaultDarkStyles）に任せると、色を無効にした設定でも lipgloss の
// カラープロファイル判定で装飾が入り、色の可否を決める箇所が 2 つになる（pane.helpStyles が
// bubbles/help を同じ理由で写している）。New でも渡すのは、最初の StateMsg が届く前に
// 描かれた入力欄に既定の配色が出ないようにするためである。
//
// 見出しは確定後の行（filterView）と同じ Muted に揃える。入力中と確定後で色が変わると、
// 確定したことが色の変化として読めてしまう。入力文字は行のセルと同じく装飾しない。
func filterStyles(s token.Styles) textinput.Styles {
	st := textinput.StyleState{
		Text:        s.Style(token.RolePlain),
		Placeholder: s.Muted,
		Suggestion:  s.Muted,
		Prompt:      s.Muted,
	}
	return textinput.Styles{
		Focused: st,
		Blurred: st,
		// カーソルの色も Styles から引く。色を無効にした Styles は NoColor を返すので装飾が
		// 入らない（反転そのものは bubbles が常に付ける。色ではなく入力位置を示す記号であり、
		// token.IconCursor と同じく色を無効にしても残す）。
		Cursor: textinput.CursorStyle{
			Color: s.Cursor.GetForeground(), Shape: tea.CursorBlock,
			Blink: true, BlinkSpeed: 0, // 0 は bubbles の既定（約 500ms）
		},
	}
}

// Update はキー入力を処理する。
func (t Model[T]) Update(msg tea.Msg) (Model[T], tea.Cmd) {
	press, ok := msg.(tea.KeyPressMsg)
	if !ok {
		// カーソルの点滅などの Msg は入力欄へ流す。Cmd を捨てると点滅が止まる。
		var cmd tea.Cmd
		t.filter, cmd = t.filter.Update(msg)
		return t, cmd
	}
	if t.filtering {
		return t.updateFiltering(press)
	}
	return t.updateList(press)
}

// updateFiltering は入力モード中のキーを処理する。
//
// 確定と取消だけを解釈し、残りはすべて入力欄へ渡す。runner 名は build01-1 のように
// 数字を含み、1〜7 を機能キーとして残すと名前で絞り込めないためである（screens.md の
// 入力中）。ctrl+c は親 Model が処理し、ここへは届かない。
func (t Model[T]) updateFiltering(press tea.KeyPressMsg) (Model[T], tea.Cmd) {
	if key.Matches(press, t.keys.Accept) {
		t.stopFiltering(false)
		return t, nil
	}
	if key.Matches(press, t.keys.Cancel) {
		t.stopFiltering(true)
		return t, nil
	}

	var cmd tea.Cmd
	t.filter, cmd = t.filter.Update(press)
	t.applyFilter()
	return t, cmd
}

// updateList は通常モードのキーを処理する。
func (t Model[T]) updateList(press tea.KeyPressMsg) (Model[T], tea.Cmd) {
	switch {
	case key.Matches(press, t.keys.Filter):
		t.filtering = true
		// 点滅の Cmd を捨てるとカーソルが出ないため、そのまま返す。
		cmd := t.filter.Focus()
		t.layout()
		return t, cmd
	case key.Matches(press, t.keys.Toggle):
		t.toggleChecked()
		return t, nil
	case key.Matches(press, t.keys.SelectAll):
		t.checkAll()
		return t, nil
	case key.Matches(press, t.keys.Down):
		if t.crossSection(1) {
			return t, nil
		}
	case key.Matches(press, t.keys.Up):
		if t.crossSection(-1) {
			return t, nil
		}
	case key.Matches(press, t.keys.Top):
		t.gotoEdge(-1)
		return t, nil
	case key.Matches(press, t.keys.Bottom):
		t.gotoEdge(1)
		return t, nil
	}
	return t.delegate(press)
}

// delegate はキーをフォーカス中の区画にのみ流す。bubbles/table は焦点の無い Model で
// Update を即座に返すため、フォーカス制御は Focus / Blur だけで成立する。
func (t Model[T]) delegate(msg tea.Msg) (Model[T], tea.Cmd) {
	if t.focus < 0 || t.focus >= len(t.sections) {
		return t, nil
	}

	var cmd tea.Cmd
	t.sections[t.focus].tbl, cmd = t.sections[t.focus].tbl.Update(msg)
	// カーソルが動いた後の位置で行を作り直し、カーソル記号を追随させる。
	t.refresh(t.focus)
	return t, cmd
}

// View は絞り込みの行と各区画を縦に並べて返す。
//
// 行が 1 つも無い場合は空文字を返す。「実行中のジョブがありません」のような文言は
// 画面ごとに変わるため page が出す。高さを超える分を末尾から落とすのは、区画の
// 最低行数の合計が高さを超える場合（layout を参照）も領域から出ないようにするため。
func (t Model[T]) View() string {
	parts := make([]string, 0, len(t.sections)*2+1)
	if t.filterVisible() {
		parts = append(parts, t.filterView())
	}
	for _, i := range t.visibleSections() {
		s := &t.sections[i]
		if s.def.Title != "" {
			parts = append(parts, atom.Divider(t.width, s.def.Title, t.styles))
		}
		parts = append(parts, s.tbl.View())
	}

	out := strings.Join(parts, "\n")
	if lines := strings.Split(out, "\n"); t.height > 0 && len(lines) > t.height {
		return strings.Join(lines[:t.height], "\n")
	}
	return out
}
