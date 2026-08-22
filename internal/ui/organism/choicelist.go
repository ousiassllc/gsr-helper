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

// Choice は 1 項目。可否と理由は page が決め、ChoiceList は判断しない。
//
// 実行できない項目も消さずに残す。消すと「押せない操作」と「存在しない操作」を
// 区別できなくなる（screens.md の無効な操作の表示）。
type Choice struct {
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
type ChosenMsg struct {
	Key string
}

// NewChoiceList は選択肢の表示を組み立てる。
func NewChoiceList(keys keymap.List, s token.Styles) ChoiceList {
	return ChoiceList{items: nil, cursor: 0, keys: keys, styles: s, width: 0}
}

// SetItems は項目を差し替え、カーソルを先頭（安全側）へ戻す。
//
// 詳細を開き直すたびに安全側へ戻す規則（FR-46）をここで担保する。一覧の enter → 詳細の
// enter で破壊的操作に到達しないようにするためである。
func (c *ChoiceList) SetItems(items []Choice) {
	c.items = items
	c.cursor = 0
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
			lines = append(lines, atom.Divider(c.width, "", c.styles))
		}
		row := molecule.ActionRow(molecule.ActionView{
			Key:         item.Key,
			Desc:        item.Desc,
			Impact:      item.Impact,
			Reason:      item.Reason,
			Enabled:     item.Enabled,
			Destructive: destructive,
		}, c.width, c.styles)
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

	chosen := ChosenMsg{Key: c.items[i].Key}
	return func() tea.Msg { return chosen }
}
