// Package organism はカーソル・選択・スクロール・入力などのローカル状態を持つ
// 部品を提供する。
//
// このパッケージ本体には選択の一覧（ChoiceList）を置く。区画に分かれた一覧
// （Table）は organism/table、スクロールする表示専用の領域（Detail / Help）は
// organism/pane に分けてある。承認・待機・入力のダイアログ（Confirm / DiffApproval /
// DrainWaiter / Form）は organism/dialog に置く。これらは互いに import せず、必要な
// ものを選んで組み合わせるのは page の役割である。
//
// この階層の型は tea.Model を実装せず、bubbles 流の「具体型を返す Update と
// View() string」に揃える。Update の戻りを tea.Model に潰すと呼び出し側で毎回型
// アサーションが必要になって panic の経路が増え、View() を tea.View にすると
// organism を縦に並べるたびに文字列へ戻す処理が入るためである（「interface は
// Executor / doctor.Check / tea.Model の 3 つに限る」規約は page と親 Model が満たす）。
// スクロール・計時・テキスト入力は自前で実装せず bubbles に委ねる。
package organism

import (
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/ui/atom"
	"github.com/ousiassllc/gsr-helper/internal/ui/keymap"
	"github.com/ousiassllc/gsr-helper/internal/ui/molecule"
	"github.com/ousiassllc/gsr-helper/internal/ui/token"
)

// cursorWidth は行頭のカーソル記号と空白 1 つが使う幅。行の幅から差し引く。
const cursorWidth = 2

// Choice は 1 項目。可否と理由は page が決め、ChoiceList は判断しない。
//
// 実行できない項目も消さずに残す。消すと「押せない操作」と「存在しない操作」を
// 区別できなくなる（screens.md の無効な操作の表示）。
//
// Impact と Reason は両方与えてよい。実際にどちらを描くかは Enabled で決まる
// （molecule.ActionView の契約。無効な項目では影響を出さず理由だけを出す）。
type Choice struct {
	// ID は呼び出し側が付ける不透明な識別子。決定（ChosenMsg）にそのまま載る。
	//
	// キーではなくこれで決定を識別するのは、キー文字列で往復させると呼び出し側が
	// 「識別子 → キー → 識別子」と再マップすることになり、キーを差し替えたときに
	// 決定が黙って別の操作へ移りうるためである。ChoiceList は中身を解釈しない。
	ID            string
	Key           string // 直接打てるキー。空なら無し
	Desc          string // 動作の説明
	Impact        string // 影響の併記（「⚠ 実行中のジョブは中断されます」など）
	Reason        string // 実行不可の理由
	Enabled       bool   // 実行できるか
	DividerBefore bool   // この項目の前に区切り線を置く（破壊的操作の区切り）
}

// ChoiceList は選択肢を並べて 1 つ選ぶ表示。詳細画面の操作リスト・反映方法の選択・
// Setup のメニューはすべてこれを使い、選択肢を並べる UI を他に作らない（atomic-design.md）。
type ChoiceList struct {
	items  []Choice
	cursor int
	keys   keymap.List
	styles token.Styles
	width  int
}

// ChosenMsg は選ばれた項目を page へ通知する。無効な項目では発行しない
// （可否は判断せず、page が与えた Enabled に従う）。
//
// 決定を識別するのは ID である。Key は「どのキーで選ばれたか」を伝えるだけで、
// 決定の同一性には使わない（Choice.ID の doc）。
type ChosenMsg struct {
	ID  string
	Key string
}

// NewChoiceList は選択肢の表示を組み立てる。
func NewChoiceList(keys keymap.List, s token.Styles) ChoiceList {
	return ChoiceList{items: nil, cursor: 0, keys: keys, styles: s, width: 0}
}

// CursorPolicy は項目を差し替えるときのカーソルの扱い。
//
// 差し替えの意味を**引数で必ず宣言させる**ために置く。以前は SetItems（先頭へ戻す）と
// UpdateItems（位置を保つ）の 2 つのメソッドを並べていたが、名前だけでは取り違えが
// 防げず、誤ると FR-46（一覧の enter → 詳細の enter で破壊的操作に到達しない）が
// 黙って崩れる。**ゼロ値は安全側（ResetCursor）である。**
type CursorPolicy int

// CursorPolicy の取り得る値。
const (
	// ResetCursor はカーソルを先頭（安全側）へ戻す。対象そのものを差し替えるときに使う。
	//
	// 詳細を開き直すたびに安全側へ戻す規則（FR-46）をここで担保する。
	ResetCursor CursorPolicy = iota
	// KeepCursor はカーソル位置を保つ。同じ対象の内容だけが変わったときに使う。
	//
	// 3 秒ごとの再検出でジョブが始まった等。先頭へ戻すと、操作を選んでいる途中で
	// 選択がずれる。件数が減った場合は末尾へ丸める。
	KeepCursor
)

// SetItems は項目を差し替える。カーソルの扱いは policy で宣言する。
func (c *ChoiceList) SetItems(items []Choice, policy CursorPolicy) {
	c.items = items
	if policy == KeepCursor {
		c.cursor = min(max(c.cursor, 0), max(len(items)-1, 0))
		return
	}
	c.cursor = 0
}

// Restyle は配色とキー定義を差し替える。項目とカーソル位置は保つ。
//
// 作り直さずに差し替えるのは、共有状態が 3 秒ごとに配られるためである。毎回
// 作り直すとカーソルが先頭へ戻り、操作を選べない。
func (c *ChoiceList) Restyle(keys keymap.List, s token.Styles) {
	c.keys, c.styles = keys, s
}

// SetWidth は 1 行の幅を設定する。実行不可の理由を右端に出すために使う。
func (c *ChoiceList) SetWidth(w int) {
	c.width = w
}

// Update はカーソル移動と実行のキーを処理する。
func (c ChoiceList) Update(msg tea.Msg) (ChoiceList, tea.Cmd) {
	press, ok := msg.(tea.KeyPressMsg)
	if !ok || len(c.items) == 0 {
		return c, nil
	}

	switch {
	case key.Matches(press, c.keys.Up):
		// 無効な項目も飛ばさずに止まる。理由を読めるようにするためである。
		c.cursor = max(c.cursor-1, 0)
		return c, nil
	case key.Matches(press, c.keys.Down):
		c.cursor = min(c.cursor+1, len(c.items)-1)
		return c, nil
	case key.Matches(press, c.keys.Accept):
		return c, c.chose(c.cursor)
	}
	return c, c.chose(c.find(press.String()))
}

// View は項目を縦に並べて返す。
func (c ChoiceList) View() string {
	lines := make([]string, 0, len(c.items)*2)
	destructive := false
	for i, item := range c.items {
		if item.DividerBefore {
			// 区切り線より下は破壊的な操作の区画として扱う。
			destructive = true
			// 各行と同じだけ字下げする。左端から引くと区切り線だけが行より
			// 外へ出て、区切りが操作リストの一部に見えない（screens.md の詳細画面）。
			lines = append(lines, strings.Repeat(" ", cursorWidth)+
				atom.Divider(c.width-cursorWidth, "", c.styles))
		}
		// カーソル記号の分を引く。引かないと理由を右端へ寄せた行が幅を超える。
		row := molecule.ActionRow(molecule.ActionView{
			Key:         item.Key,
			Desc:        item.Desc,
			Impact:      item.Impact,
			Reason:      item.Reason,
			Enabled:     item.Enabled,
			Destructive: destructive,
		}, max(c.width-cursorWidth, 0), c.styles)
		lines = append(lines, atom.Cursor(i == c.cursor, c.styles)+" "+row)
	}
	return strings.Join(lines, "\n")
}

// Cursor はカーソル位置を返す。
func (c ChoiceList) Cursor() int { return c.cursor }

// find はキーに一致する項目の位置を返す。無ければ -1 を返す。
//
// 詳細画面では操作キーを直接打てる（screens.md の詳細画面）。判定をここに置くのは、
// カーソルで選ぶ経路と直接打つ経路で可否の扱いを揃えるためである。
func (c ChoiceList) find(pressed string) int {
	for i, item := range c.items {
		if item.Key != "" && item.Key == pressed {
			return i
		}
	}
	return -1
}

// chose は項目を選んだことを通知する Cmd を返す。無効な項目では何も返さない。
func (c ChoiceList) chose(i int) tea.Cmd {
	if i < 0 || i >= len(c.items) || !c.items[i].Enabled {
		return nil
	}

	chosen := ChosenMsg{ID: c.items[i].ID, Key: c.items[i].Key}
	return func() tea.Msg { return chosen }
}
