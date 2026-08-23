package setup

import (
	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/runner"
	"github.com/ousiassllc/gsr-helper/internal/ui/organism"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
)

// 画面の見出しとメニュー項目の識別子。識別子は organism.Choice.ID に載せて
// 決定（organism.ChosenMsg）で戻ってくる不透明な値である。
const (
	headMenu = "runner の追加"

	idBulk   = "setup.bulk"
	idWizard = "setup.wizard"
	idUpdate = "setup.update"
)

// 状態行に出す案内。
const (
	noticeDeleteFromList = "削除は Runners タブで対象を選び D を押してください"
	noticeNoRunner       = "対象の runner がありません"
	noticeRunning        = "実行中です。完了までお待ちください"
)

// setMenu はメニューの項目を組み直す。
//
// 可否と理由は action.Allow から引く。同じ判定をこの画面で書き直すと、Runners
// タブのフッタと Setup のメニューで理由が食い違う（screens.md「無効な操作の表示」）。
func (m *Model) setMenu(policy organism.CursorPolicy) {
	addOK, addWhy := m.allowed(page.BindingKey(m.st.Keys.Runner.Add))
	upOK, upWhy := m.allowed(page.BindingKey(m.st.Keys.Runner.Update))

	// 対象が 1 台も無ければ更新は選べない。押しても何も起きない項目を作らない。
	if upOK && len(m.allRunners()) == 0 {
		upOK, upWhy = false, noticeNoRunner
	}

	m.menu.SetItems([]organism.Choice{
		{
			ID: idBulk, Key: "", Desc: "台数を指定して一括追加", Impact: "",
			Reason: addWhy, Enabled: addOK, DividerBefore: false,
		},
		{
			ID: idWizard, Key: "", Desc: "1 台ずつ個別に設定して追加", Impact: "",
			Reason: addWhy, Enabled: addOK, DividerBefore: false,
		},
		{
			ID: idUpdate, Key: "", Desc: "バージョンを一括更新", Impact: "",
			Reason: upWhy, Enabled: upOK, DividerBefore: false,
		},
	}, policy)
}

// allowed はキー 1 つの可否と理由を返す。
//
// 対象の runner を持たない操作（追加・全台更新）なのでゼロ値を渡す。判定表のうち
// この画面に効くのは root と認証の 2 行だけで、どちらも runner を見ない。
func (m Model) allowed(k string) (bool, string) {
	return m.actions.Allowed(k, runner.Runner{}, m.st.Caps)
}

// onChosen はメニューの決定を処理する。
func (m *Model) onChosen(msg organism.ChosenMsg) tea.Cmd {
	if m.run != nil {
		m.notice = noticeRunning
		return nil
	}

	switch msg.ID {
	case idBulk:
		return m.startAdd(bulkForm)
	case idWizard:
		return m.startAdd(wizardForm)
	case idUpdate:
		return m.startUpdate(m.allRunners())
	default:
		return nil
	}
}

// allRunners は検出済みの runner をすべて返す（FR-20 の「全台」）。
func (m Model) allRunners() []runner.Runner { return m.st.Result.Runners }
