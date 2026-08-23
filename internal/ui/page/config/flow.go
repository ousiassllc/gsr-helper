package config

import (
	"context"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/exec"
	"github.com/ousiassllc/gsr-helper/internal/gh"
	"github.com/ousiassllc/gsr-helper/internal/runner"
	"github.com/ousiassllc/gsr-helper/internal/ui/organism"
	"github.com/ousiassllc/gsr-helper/internal/ui/organism/dialog"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
)

// selfID は対象の一覧に出す「本ツール自身の設定」の識別子。
// runner 名と衝突しないよう、runner 名に使えない文字を含める。
const selfID = "/self"

// loadedMsg は API から取った値。フォームを開く前に届く。
type loadedMsg struct {
	kind   kind
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
	case m.overlay.Active():
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

// goBack は esc を処理する。対象を選んでいれば対象の選択へ、そうでなければ
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

// wrap は本文が返した Cmd の結果をこのタブへ戻す（page/setup の wrapMenu と同じ）。
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
	out := make([]runner.Runner, 0, len(m.st.Result.Runners))
	for _, r := range m.st.Result.Runners {
		if r.Dir != m.target.Dir {
			out = append(out, r)
		}
	}
	return out
}

// containsFold は大文字小文字を無視して含むかを返す。
func containsFold(s, q string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(q))
}

// commitInputOf は書き込みに渡す値を組み立てる。
func (m Model) commitInputOf(c change) commitInput {
	ex := m.st.Exec
	return commitInput{
		change: c, runner: m.target,
		client: func(ctx context.Context) (*gh.Client, error) { return m.newClient(ctx, ex) },
	}
}

// approve は差分の承認を開く（FR-37 / FR-38）。
func (m *Model) approve(c change) tea.Cmd {
	if !c.changed() {
		m.notice = "変更はありません"
		m.overlay.Close()

		return nil
	}

	m.pending = c
	m.overlay.Close()

	return m.overlay.Open(diffKind, diffOpenMsg{input: dialog.DiffApprovalInput{
		Path: c.title(), Diff: c.diffLines(), Backup: c.backupPath(),
	}})
}
