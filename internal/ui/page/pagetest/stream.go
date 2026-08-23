package pagetest

import (
	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/ui/page"
)

// 長寿命の処理を持つタブを模した page を置く。親 Model の寿命の通知（Issue #41）を
// 検証する側が使う。

// CleanupDoneMsg は後始末の Cmd が実行されたことを表す。
type CleanupDoneMsg struct{ Tab int }

// StreamPage は長寿命の購読（journalctl -f のようなストリーム）を持つ page を
// 模したテスト用の Model。前面に出たら 1 本張り、裏へ回ったら畳む。
//
// 購読は 0 本から始まる。**起動時に選択されているタブも page.ActivateMsg を受け取る**
// ので（Issue #63）、親へ最初の共有状態を配った時点で既定タブは 1 本になる。
type StreamPage struct {
	Tab       int
	Open      int // 開いている購読の本数
	Peak      int // 同時に開いた最大本数（積み上がりの検出に使う）
	Stops     int // 後始末の Cmd が実行された回数
	Shutdowns int // 終了の通知を受けた回数
}

// tea.Model を実装していることをコンパイル時に確かめる。
var _ tea.Model = (*StreamPage)(nil)

// NewStreamPage は購読をまだ張っていない page を返す。
func NewStreamPage(tab int) *StreamPage {
	return &StreamPage{Tab: tab, Open: 0, Peak: 0, Stops: 0, Shutdowns: 0}
}

// Init は何も発行しない。
func (s *StreamPage) Init() tea.Cmd { return nil }

// Update は寿命の通知で購読を張り直し、キーは親へ差し戻す。
func (s *StreamPage) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case page.ActivateMsg:
		s.Open++
		s.Peak = max(s.Peak, s.Open)
		return s, nil
	case page.DeactivateMsg:
		return s, s.close()
	case page.ShutdownMsg:
		s.Shutdowns++
		return s, s.close()
	case tea.KeyPressMsg:
		return s, page.BubbleKey(msg)
	default:
		return s, nil
	}
}

// View は本体の代わりに固定の文字列を返す。
func (s *StreamPage) View() tea.View { return tea.NewView("stream") }

// close は購読を 1 本畳み、後始末の Cmd を返す。開いていなければ何もしない。
func (s *StreamPage) close() tea.Cmd {
	if s.Open == 0 {
		return nil
	}
	s.Open--
	tab := s.Tab
	return func() tea.Msg {
		s.Stops++
		return CleanupDoneMsg{Tab: tab}
	}
}
