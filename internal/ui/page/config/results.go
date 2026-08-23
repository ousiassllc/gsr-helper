package config

import (
	"context"
	"errors"

	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/config/apply"
	"github.com/ousiassllc/gsr-helper/internal/ui/organism"
	"github.com/ousiassllc/gsr-helper/internal/ui/organism/dialog"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
)

// openSelected はカーソル位置の項目の編集を始める。
func (m *Model) openSelected() tea.Cmd {
	it, ok := m.list.Selected()
	if !ok || !it.enabled {
		return nil
	}

	switch it.kind {
	case kindLabels, kindGroup:
		return m.fetchFor(it.kind)
	case kindEnv, kindPath, kindDropIn, kindCopy:
		return m.openForm(it.kind)
	case kindReregister:
		m.notice = "変更には再登録が必要です（Setup タブで削除して追加し直してください）"
		return nil
	default:
		return nil
	}
}

// fetchFor は GitHub 側の現在値を取ってからフォームを開く。
//
// 一覧の組み立てでは API を呼ばず、項目を選んだこの時点で呼ぶ
// （3 秒ポーリングで API を呼ばない方針。docs/api/external-interfaces.md）。
func (m *Model) fetchFor(k kind) tea.Cmd {
	m.busy = true
	in := m.commitInputOf(change{})

	return page.Do(m.tab, func() tea.Msg {
		ctx := context.Background()
		if k == kindLabels {
			labels, err := fetchLabels(ctx, in)
			return loadedMsg{kind: k, labels: labels, groups: nil, err: err}
		}

		groups, err := fetchGroups(ctx, in)
		return loadedMsg{kind: k, labels: nil, groups: groups, err: err}
	})
}

// onLoaded は取得した現在値をフォームへ入れて開く。
func (m *Model) onLoaded(msg loadedMsg) tea.Cmd {
	m.busy = false
	if msg.err != nil {
		m.notice = msg.err.Error()
		return nil
	}

	if msg.kind == kindLabels {
		m.vals.labels = joinLabels(msg.labels)
		return m.openForm(kindLabels)
	}

	m.vals.groups = m.vals.groups[:0]
	m.vals.groupIDs = m.vals.groupIDs[:0]
	for _, g := range msg.groups {
		m.vals.groups = append(m.vals.groups, g.Name)
		m.vals.groupIDs = append(m.vals.groupIDs, g.ID)
	}
	return m.openForm(kindGroup)
}

// openForm は種類に応じてフォームの初期値を入れて開く。
func (m *Model) openForm(k kind) tea.Cmd {
	m.vals.kind = k
	if err := m.fillForm(k); err != nil {
		m.notice = err.Error()
		return nil
	}

	m.formShown = true

	return m.overlay.Open(formKind, formOpenMsg{title: formTitleOf(k), values: m.vals, st: m.st})
}

// fillForm はフォームの初期値を現在の設定から入れる。
func (m *Model) fillForm(k kind) error {
	switch k {
	case kindEnv:
		f, err := m.ld.env(m.target)
		if err != nil {
			return err
		}
		for i, spec := range envKeys {
			v, _ := f.Get(spec.key)
			m.vals.env[i], m.vals.envBefore[i] = v, v
		}
		return nil
	case kindPath:
		p, err := m.ld.pathFile(m.target)
		if err != nil {
			return err
		}
		m.vals.path = p.Value
		return nil
	case kindDropIn:
		d, err := m.ld.dropIn(m.target)
		if err != nil {
			return err
		}
		m.vals.restart, _ = d.Get("Restart")
		m.vals.memoryMax, _ = d.Get("MemoryMax")
		return nil
	case kindCopy:
		m.vals.copyTo = nil
		m.vals.copyCandidates = names(m.others())
		return nil
	case kindLabels, kindGroup, kindReregister:
		return nil
	default:
		return nil
	}
}

// onResult はモーダルの決定を処理する。
func (m *Model) onResult(msg page.ResultMsg) tea.Cmd {
	switch msg.Kind {
	case formKind:
		return m.onForm(msg.Msg)
	case diffKind:
		return m.onApproved(msg.Msg)
	case applyKind:
		return m.onApplyChosen(msg.Msg)
	default:
		return nil
	}
}

// onForm はフォームの完了・中断を処理する。
func (m *Model) onForm(msg tea.Msg) tea.Cmd {
	if _, done := msg.(dialog.FormDoneMsg); !done {
		// 中断と破棄はフォームを閉じるだけ。
		m.formShown = false
		m.overlay.Close()

		return nil
	}
	m.formShown = false

	if m.self {
		return m.saveSelf()
	}

	c, err := m.buildChange()
	if err != nil {
		m.notice = err.Error()
		m.overlay.Close()

		return nil
	}
	return m.approve(c)
}

// buildChange はフォームの入力から変更を組み立てる。
func (m Model) buildChange() (change, error) {
	switch m.vals.kind {
	case kindEnv:
		return buildEnv(m.ld, m.target, m.vals)
	case kindPath:
		return buildPath(m.ld, m.target, m.vals)
	case kindDropIn:
		return buildDropIn(m.ld, m.target, m.vals)
	case kindLabels:
		return m.buildLabelChange()
	case kindGroup:
		id, ok := m.vals.groupID()
		if !ok {
			return change{}, ErrUnknownGroup
		}
		return buildGroup("", m.vals.group, id), nil
	case kindCopy:
		return buildCopy(m.ld, m.target, m.others(), m.vals)
	case kindSelf, kindReregister:
		return change{}, errNoUnit
	default:
		return change{}, errNoUnit
	}
}

// ErrUnknownGroup は一覧に無い runner group を指定した場合のエラー。
var ErrUnknownGroup = errors.New("runner group の ID が分からないため変更できません")

// buildLabelChange はラベルの変更を組み立てる。検証はドメイン層に委ねる。
func (m Model) buildLabelChange() (change, error) {
	after, err := validateLabelList(m.vals.labels)
	if err != nil {
		return change{}, err
	}
	return buildLabels(nil, after), nil
}

// onApproved は差分の承認を処理する。承認されたときだけ書き込む。
func (m *Model) onApproved(msg tea.Msg) tea.Cmd {
	d, ok := msg.(dialog.DecidedMsg)
	m.overlay.Close()

	if !ok || !d.Confirmed {
		m.notice = "書き込みを取りやめました"
		m.self = false

		return nil
	}

	if m.self {
		m.self = false
		return m.commitSelf()
	}

	m.busy = true
	in := m.commitInputOf(m.pending)

	return page.Do(m.tab, func() tea.Msg {
		if err := commit(context.Background(), in); err != nil {
			return doneMsg{text: "", err: err}
		}
		return doneMsg{text: "書き込みました", err: nil}
	})
}

// onDone は書き込みと反映の結果を処理する。
//
// ファイルを書き換えた場合だけ反映方法の選択へ進む（FR-39）。ラベルと
// runner group は GitHub 側で即時に反映され、再起動が要らない。
func (m *Model) onDone(msg doneMsg) tea.Cmd {
	m.busy = false
	if msg.err != nil {
		m.report = "失敗: " + msg.err.Error()
		return nil
	}
	m.report = msg.text
	m.refresh(organism.KeepCursor)

	if m.pending.fileBacked() && m.target.UnitName != "" {
		return m.overlay.Open(applyKind, applyOpenMsg{items: applyChoices()})
	}
	m.pending = change{}

	return nil
}

// applyChoices は反映方法の選択肢を返す。既定（ドレイン再起動）を先頭に置く。
func applyChoices() []organism.Choice {
	ms := apply.Methods()
	out := make([]organism.Choice, 0, len(ms))

	for _, method := range ms {
		out = append(out, organism.Choice{
			ID: method.Label(), Key: "", Desc: method.Label(), Impact: "", Reason: "",
			Enabled: true, DividerBefore: false,
		})
	}
	return out
}

// onApplyChosen は選ばれた反映方法を実行する。
func (m *Model) onApplyChosen(msg tea.Msg) tea.Cmd {
	chosen, ok := msg.(organism.ChosenMsg)
	m.overlay.Close()

	if !ok {
		return nil
	}

	method := apply.Drain
	for _, cand := range apply.Methods() {
		if cand.Label() == chosen.ID {
			method = cand
		}
	}

	in := apply.Input{
		Exec: m.st.Exec, Runner: m.target, Method: method,
		Reload: m.pending.reload, Progress: nil, Drain: nil,
	}
	m.pending = change{}
	m.busy = true

	return page.Do(m.tab, func() tea.Msg {
		if err := apply.Run(context.Background(), in); err != nil {
			return doneMsg{text: "", err: err}
		}
		return doneMsg{text: "反映しました", err: nil}
	})
}
