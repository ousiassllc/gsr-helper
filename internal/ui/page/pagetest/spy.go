package pagetest

import (
	"sync"

	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/ui/page"
)

// Spy は共有状態・キー・その他の Msg の受信を記録するテスト用の page。
//
// ポインタで tea.Model を実装するのは、Update が返す値ではなく記録そのものを
// テストから見たいためである。
//
// **記録は mutex で守り、読み出しは複製を返す。** page が発行した Cmd を別の
// goroutine で回すテスト（bubbletea のランタイムに近い形で回す場合）では Update と
// 読み出しが同時に走りうる。-race を付けた test はその最初の 1 回で落ちる
// （Issue #45）。
type Spy struct {
	Tab int // ChromeMsg に載せるタブ番号

	// Chrome は Update が返す ChromeMsg の雛形。Tab は Update が上書きする。
	//
	// **最初の Update より前に設定すること。** Update 中は読むだけである。
	Chrome page.ChromeMsg

	// Bubble が真なら、受け取ったキーを page.GlobalKeyMsg として親へ差し戻す。
	// モーダル表示中・入力中（Chrome の Modal / Input）は差し戻さない
	// （本物の page と同じ規則。page.GlobalKeyMsg の doc）。
	//
	// **親 Model（internal/ui）の検証にはこれが要る。** 差し戻さないと親はグローバル
	// キーを解釈できず、キーの配送そのものを検証できない。タブ側の検証は記録だけを
	// 見るので既定（偽）でよい。この 1 つの振る舞いのために ui 直下が同じ spy を
	// 写し持っていた（Issue #45）。
	Bubble bool

	mu     sync.Mutex
	states []page.StateMsg   // 受け取った共有状態
	keys   []tea.KeyPressMsg // 受け取ったキー
	msgs   []tea.Msg         // 上記以外（page が発行した Cmd の結果など）
}

// tea.Model を実装していることをコンパイル時に確かめる。
var _ tea.Model = (*Spy)(nil)

// NewSpy はタブ番号を持つ spy を返す。差し戻しはしない（Bubble を立てると行う）。
func NewSpy(tab int) *Spy {
	return &Spy{
		Tab:    tab,
		Chrome: page.ChromeMsg{Tab: tab, Modal: false, Input: "", Status: "", Footer: nil},
		Bubble: false,
		mu:     sync.Mutex{},
		states: nil,
		keys:   nil,
		msgs:   nil,
	}
}

// Init は何も発行しない。
func (s *Spy) Init() tea.Cmd { return nil }

// Update は受け取った Msg を記録し、自分の ChromeMsg を返す。
//
// Bubble が真で、モーダル表示中でも入力中でもないキーは、ChromeMsg と一緒に
// page.GlobalKeyMsg として差し戻す。
func (s *Spy) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	bubble := false

	s.mu.Lock()
	switch m := msg.(type) {
	case page.StateMsg:
		s.states = append(s.states, m)
	case tea.KeyPressMsg:
		s.keys = append(s.keys, m)
		bubble = s.Bubble && !s.Chrome.Modal && s.Chrome.Input == ""
	default:
		s.msgs = append(s.msgs, m)
	}
	s.mu.Unlock()

	c := s.Chrome
	c.Tab = s.Tab
	chrome := func() tea.Msg { return c }
	if press, ok := msg.(tea.KeyPressMsg); ok && bubble {
		return s, tea.Batch(chrome, page.BubbleKey(press))
	}
	return s, chrome
}

// View は本体の代わりに固定の文字列を返す。
func (s *Spy) View() tea.View { return tea.NewView("spy") }

// States は受け取った共有状態を届いた順に返す。
func (s *Spy) States() []page.StateMsg {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]page.StateMsg(nil), s.states...)
}

// Keys は受け取ったキーを届いた順に返す。
func (s *Spy) Keys() []tea.KeyPressMsg {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]tea.KeyPressMsg(nil), s.keys...)
}

// Msgs は共有状態でもキーでもない Msg を届いた順に返す。
func (s *Spy) Msgs() []tea.Msg {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]tea.Msg(nil), s.msgs...)
}
