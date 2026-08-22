package pagetest

import (
	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/ui/page"
)

// Spy は共有状態・キー・その他の Msg の受信を記録するテスト用の page。
//
// ポインタで tea.Model を実装するのは、Update が返す値ではなく記録そのものを
// テストから見たいためである。
type Spy struct {
	Tab    int               // ChromeMsg に載せるタブ番号
	States []page.StateMsg   // 受け取った共有状態
	Keys   []tea.KeyPressMsg // 受け取ったキー
	Msgs   []tea.Msg         // 上記以外（page が発行した Cmd の結果など）
	Chrome page.ChromeMsg    // Update で返す ChromeMsg
}

// tea.Model を実装していることをコンパイル時に確かめる。
var _ tea.Model = (*Spy)(nil)

// NewSpy はタブ番号を持つ spy を返す。
func NewSpy(tab int) *Spy {
	return &Spy{
		Tab:    tab,
		States: nil,
		Keys:   nil,
		Msgs:   nil,
		Chrome: page.ChromeMsg{Tab: tab, Modal: false, Input: "", Status: "", Footer: nil},
	}
}

// Init は何も発行しない。
func (s *Spy) Init() tea.Cmd { return nil }

// Update は受け取った Msg を記録し、自分の ChromeMsg を返す。
func (s *Spy) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {
	case page.StateMsg:
		s.States = append(s.States, m)
	case tea.KeyPressMsg:
		s.Keys = append(s.Keys, m)
	default:
		s.Msgs = append(s.Msgs, m)
	}

	c := s.Chrome
	c.Tab = s.Tab
	return s, func() tea.Msg { return c }
}

// View は本体の代わりに固定の文字列を返す。
func (s *Spy) View() tea.View { return tea.NewView("spy") }
