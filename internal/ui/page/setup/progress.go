package setup

import (
	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/setup"
	"github.com/ousiassllc/gsr-helper/internal/ui/atom"
	"github.com/ousiassllc/gsr-helper/internal/ui/keymap"
	"github.com/ousiassllc/gsr-helper/internal/ui/organism/pane"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
)

// progressKind は進捗表示のモーダルの種類。
const progressKind page.ModalKind = "setupprogress"

// progressOpenMsg は進捗表示を開く指示。
type progressOpenMsg struct{ input pane.ProgressInput }

// progressSetMsg は進捗表示の中身を差し替える指示。
type progressSetMsg struct{ input pane.ProgressInput }

// progressStopMsg はスピナを止める指示。
type progressStopMsg struct{}

// openProgress は進捗表示を開く。
func (m Model) openProgress(plan setup.Plan) tea.Cmd {
	return m.overlay.Open(progressKind, progressOpenMsg{input: pane.ProgressInput{
		Title:  runTitle(plan.Kind.String(), 0, len(plan.Units)),
		Rows:   waitingRows(plan),
		Done:   0,
		Total:  len(plan.Units),
		Report: nil,
	}})
}

// updateProgress は進捗表示へ現在の状態を送る。
func (m Model) updateProgress() tea.Cmd {
	if m.run == nil {
		return nil
	}

	in := pane.ProgressInput{
		Title:  m.run.title,
		Rows:   m.run.rows,
		Done:   m.run.done,
		Total:  m.run.total,
		Report: m.report,
	}
	return m.overlay.Open(progressKind, progressSetMsg{input: in})
}

// progressModal は進捗表示。pane.ProgressList を包むだけで判断は持たない。
type progressModal struct {
	tab     int
	list    pane.ProgressList
	keys    keymap.Set
	running bool
}

// tea.Model を実装していることをコンパイル時に確かめる。
var _ tea.Model = progressModal{}

// newProgressModal は進捗表示のモーダルを組み立てる。
func newProgressModal(st page.StateMsg) page.Modal {
	return page.Modal{
		Model: progressModal{
			tab: 0, list: pane.NewProgressList(st.Styles), keys: st.Keys, running: false,
		},
		Title: progressTitle,
		Hints: progressHints,
		// 実行中は esc で閉じさせない。閉じても処理は止まらないため、進捗を
		// 見失うだけになる。中止は明示的なキャンセルで行う。
		HandlesBack: progressHandlesBack,
	}
}

// Init は何も発行しない。
func (m progressModal) Init() tea.Cmd { return nil }

// Update は進捗表示へ Msg を配る。
func (m progressModal) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case page.AttachMsg:
		m.tab = msg.Tab
		return m, nil
	case progressOpenMsg:
		m.list.SetInput(msg.input)
		m.running = true
		return m, page.WrapModal(m.tab, progressKind, m.list.Start())
	case progressSetMsg:
		m.list.SetInput(msg.input)
		return m, nil
	case progressStopMsg:
		m.running = false
		return m, page.WrapModal(m.tab, progressKind, m.list.Stop())
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
		return m, page.WrapModal(m.tab, progressKind, cmd)
	}
}

// View は進捗表示の中身を返す。
func (m progressModal) View() tea.View { return tea.NewView(m.list.View()) }

// progressTitle はモーダルの見出しを返す。
func progressTitle(tea.Model) string { return "実行中" }

// progressHandlesBack は実行中の esc を握りつぶすかを返す。
func progressHandlesBack(model tea.Model) bool {
	m, ok := model.(progressModal)
	return ok && m.running
}

// progressHints はモーダルのフッタを返す。
func progressHints(model tea.Model) []atom.Hint {
	m, ok := model.(progressModal)
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
