package config

import (
	"context"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/config/edit"
	"github.com/ousiassllc/gsr-helper/internal/exec"
	"github.com/ousiassllc/gsr-helper/internal/gh"
	"github.com/ousiassllc/gsr-helper/internal/runner"
	"github.com/ousiassllc/gsr-helper/internal/ui/organism"
	"github.com/ousiassllc/gsr-helper/internal/ui/organism/dialog"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/configmodal"
)

// selfID は対象の一覧に出す「本ツール自身の設定」の識別子。
// runner 名と衝突しないよう、runner 名に使えない文字を含める。
const selfID = "/self"

// loadedMsg は API から取った値。フォームを開く前に届く。
type loadedMsg struct {
	kind edit.Kind
	// dir は取得を始めた時点の対象。届いたときに対象が変わっていたら捨てる
	// （fetchFor の doc）。
	dir    string
	labels []string
	groups []gh.RunnerGroup
	err    error
}

// doneMsg は書き込みまたは反映の結果。
type doneMsg struct {
	text string
	err  error
}

// defaultClient は GitHub API のクライアントを作る。
func defaultClient(ctx context.Context, ex exec.Executor) (*gh.Client, error) {
	tok, err := gh.Token(ctx, ex)
	if err != nil {
		return nil, err
	}
	return gh.New(tok)
}

// handleKey はキー入力を処理する。
func (m Model) handleKey(press tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	m.notice = ""

	var cmd tea.Cmd
	switch {
	// 絞り込みの入力中とモーダル表示中は**親へ差し戻さない**。差し戻すと、
	// 絞り込みに打った q でアプリが終わり、数字でタブが切り替わる
	// （page.GlobalKeyMsg の doc、page/runners の handleKey と同じ判定）。
	// enter / esc をここで解釈しないのも同じ理由で、絞り込みの確定と取消が
	// 「編集を開く」「対象を捨てる」に化けないようにする。
	case m.filtering(), m.overlay.Active():
		return m.forwardTo(press)
	case key.Matches(press, m.st.Keys.Global.Help):
		cmd = m.overlay.OpenHelp()
	case m.target.Dir != "" && key.Matches(press, m.st.Keys.List.Enter):
		cmd = m.openSelected()
	case key.Matches(press, m.st.Keys.Global.Back):
		return m.goBack()
	default:
		next, c := m.forwardTo(press)
		return next, tea.Batch(c, page.BubbleKey(press))
	}
	return m, tea.Batch(m.chrome(), cmd)
}

// goBack は esc を処理する。対象を選んでいれば対象の選択へ、無ければ
// Runners タブへ戻る（戻り先が一意に決まるので自分で移動を要求する）。
func (m Model) goBack() (tea.Model, tea.Cmd) {
	if m.target.Dir != "" {
		m.target = runner.Runner{}
		m.report = ""
		m.refresh(organism.ResetCursor)

		return m, m.chrome()
	}
	return m, tea.Batch(m.chrome(), page.OpenTab(page.TabRunners, nil))
}

// filtering は本文の一覧で絞り込みを入力中かを返す。
//
// 対象を選ぶ前の画面（picker）は絞り込みを持たないので、対象が決まっている
// ときだけ見る。
func (m Model) filtering() bool { return m.target.Dir != "" && m.list.Filtering() }

// forwardTo はモーダルか本文へ Msg を配る。
func (m Model) forwardTo(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	if m.overlay.Handles(msg) {
		m.overlay, cmd = m.overlay.Update(msg)
		return m, tea.Batch(m.chrome(), cmd)
	}

	if m.target.Dir == "" {
		m.picker, cmd = m.picker.Update(msg)
		return m, tea.Batch(m.chrome(), m.wrap(cmd))
	}

	m.list, cmd = m.list.Update(msg)
	return m, tea.Batch(m.chrome(), m.wrap(cmd))
}

// wrap は本文の Cmd の結果をこのタブへ戻す（page/setup の wrapMenu と同じ）。
func (m Model) wrap(cmd tea.Cmd) tea.Cmd {
	if cmd == nil {
		return nil
	}
	return page.Do(m.tab, func() tea.Msg { return cmd() })
}

// pickerItems は対象の一覧を組み立てる。
func (m Model) pickerItems() []organism.Choice {
	rs := m.st.Result.Runners
	out := make([]organism.Choice, 0, len(rs)+1)

	for _, r := range rs {
		out = append(out, organism.Choice{
			ID: r.Name(), Key: "", Desc: r.Name(), Impact: "", Reason: "",
			Enabled: true, DividerBefore: false,
		})
	}

	out = append(out, organism.Choice{
		ID: selfID, Key: "", Desc: "gsr-helper 自身の設定", Impact: "", Reason: "",
		Enabled: true, DividerBefore: len(rs) > 0,
	})
	return out
}

// onPicked は対象の選択を処理する。
func (m *Model) onPicked(msg organism.ChosenMsg) tea.Cmd {
	if msg.ID == selfID {
		return m.openSelfForm()
	}

	for _, r := range m.st.Result.Runners {
		if r.Name() == msg.ID {
			m.setTarget(r)
			return nil
		}
	}
	return nil
}

// setTarget は編集する runner を決める。
func (m *Model) setTarget(r runner.Runner) {
	m.target = r
	m.report = ""
	m.self = false
	m.refresh(organism.ResetCursor)
}

// others は複製先の候補（対象以外の runner）を返す。
func (m Model) others() []runner.Runner {
	return edit.OtherRunners(m.st.Result.Runners, m.target.Dir)
}

// commitInputOf は書き込みに渡す値を組み立てる。
func (m Model) commitInputOf(c edit.Change) edit.CommitInput {
	ex := m.st.Deps.Exec
	return edit.CommitInput{
		Change: c, Runner: m.target,
		Client: func(ctx context.Context) (*gh.Client, error) { return m.newClient(ctx, ex) },
	}
}

// approve は差分の承認を開く（FR-37 / FR-38）。
func (m *Model) approve(c edit.Change) tea.Cmd {
	if !c.Changed() {
		m.notice = "変更はありません"
		m.overlay.Close()

		return nil
	}

	m.pending, m.pendingSet = c, true
	m.overlay.Close()

	return configmodal.OpenDiff(&m.overlay, dialog.DiffApprovalInput{
		Path: c.Title(), Diff: c.DiffLines(), Backup: c.BackupPath(),
	})
}
