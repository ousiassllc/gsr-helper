// Package progressmodal は一括処理の逐次表示と結果報告のモーダル（FR-15）を提供する。
//
// Setup タブと Disk タブが共有する。先例は internal/ui/page/runnerdetail と
// internal/ui/page/runnerop（タブではなく、複数の page が使うサブパッケージ）。
// この種の進捗表示に「見出しに件数をどう添えるか」「対象ごとの行をどう作るか」
// といったタブ固有の判断は無く、pane.ProgressList を page.Modal として組み立て、
// 開く・差し替える・スピナを止める、を提供するだけである。タブ固有の判断
// （見出しの文言・行の内容・完了後の報告文）は呼び出し側の page が持つ。
package progressmodal

import (
	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/ui/atom"
	"github.com/ousiassllc/gsr-helper/internal/ui/keymap"
	"github.com/ousiassllc/gsr-helper/internal/ui/organism/pane"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
)

// Kind は進捗表示のモーダルの種類。画面が page.Overlay へ登録するときに使う。
const Kind page.ModalKind = "progress"

// OpenMsg は進捗表示を開く指示。
type OpenMsg struct{ Input pane.ProgressInput }

// SetMsg は進捗表示の中身を差し替える指示。
type SetMsg struct{ Input pane.ProgressInput }

// StopMsg はスピナを止める指示。
type StopMsg struct{}

// Open は進捗表示を開く。Setup / Disk タブが共用する入口である。
//
// 種類と Msg の組を画面ごとに書かせないために用意する（runnerdetail.Open と同じ
// 理由）。戻り値の Cmd は呼び出し側まで返すこと。捨てると、開いた瞬間にスピナを
// 動かす Cmd が失われる（page.Overlay.Open の doc）。
func Open(o *page.Overlay, input pane.ProgressInput) tea.Cmd {
	return o.Open(Kind, OpenMsg{Input: input})
}

// Set は進捗表示の中身を差し替える。逐次の完了通知が届くたびに呼ぶ。
//
// 開き直すのではなく Open を使い回すのは、Overlay の push が同じ種類を二重に
// 積まない（既にあれば最前面へ移すだけ）ためである。進捗 1 件ごとに呼ぶので、
// 積み上がる実装だと数百枚のモーダルが溜まる。
func Set(o *page.Overlay, input pane.ProgressInput) tea.Cmd {
	return o.Open(Kind, SetMsg{Input: input})
}

// Stop はスピナを止める。実行が終わった後、止め忘れると Tick が流れ続ける。
func Stop(o *page.Overlay) tea.Cmd {
	return o.Open(Kind, StopMsg{})
}

// modal は進捗表示。pane.ProgressList を包むだけで判断は持たない。
type modal struct {
	tab     int
	list    pane.ProgressList
	keys    keymap.Set
	running bool
}

// tea.Model を実装していることをコンパイル時に確かめる。
var _ tea.Model = modal{}

// New は進捗表示のモーダルを組み立てる。画面は page.Overlay.Register に渡す。
func New(st page.StateMsg) page.Modal {
	return page.Modal{
		Model: modal{
			tab: 0, list: pane.NewProgressList(st.Styles), keys: st.Keys, running: false,
		},
		Title: title,
		Hints: hints,
		// 実行中は esc で閉じさせない。閉じても処理は止まらないため、進捗を
		// 見失うだけになる。中止は明示的なキャンセルで行う。
		HandlesBack: handlesBack,
	}
}

// Init は何も発行しない。
func (m modal) Init() tea.Cmd { return nil }

// Update は進捗表示へ Msg を配る。
func (m modal) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case page.AttachMsg:
		m.tab = msg.Tab
		return m, nil
	case OpenMsg:
		m.list.SetInput(msg.Input)
		m.running = true
		return m, page.WrapModal(m.tab, Kind, m.list.Start())
	case SetMsg:
		m.list.SetInput(msg.Input)
		return m, nil
	case StopMsg:
		m.running = false
		return m, page.WrapModal(m.tab, Kind, m.list.Stop())
	case page.StateMsg:
		m.list.Restyle(msg.Styles)
		m.keys = msg.Keys
		return m, nil
	case page.SizeMsg:
		m.list.SetSize(msg.W, msg.H)
		return m, nil
	default:
		var cmd tea.Cmd
		m.list, cmd = m.list.Update(msg)
		return m, page.WrapModal(m.tab, Kind, cmd)
	}
}

// View は進捗表示の中身を返す。
func (m modal) View() tea.View { return tea.NewView(m.list.View()) }

// title はモーダルの見出しを返す。
func title(tea.Model) string { return "実行中" }

// handlesBack は実行中の esc を握りつぶすかを返す。
func handlesBack(model tea.Model) bool {
	m, ok := model.(modal)
	return ok && m.running
}

// hints はモーダルのフッタを返す。
func hints(model tea.Model) []atom.Hint {
	m, ok := model.(modal)
	if !ok {
		return nil
	}
	if m.running {
		return []atom.Hint{{Key: "", Desc: "実行中…", Enabled: false, Reason: ""}}
	}
	return []atom.Hint{{
		Key:     page.BindingKey(m.keys.Global.Back),
		Desc:    "閉じる",
		Enabled: true,
		Reason:  "",
	}}
}
