package config

import (
	"context"

	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/appconfig"
	"github.com/ousiassllc/gsr-helper/internal/config"
	"github.com/ousiassllc/gsr-helper/internal/config/edit"
	"github.com/ousiassllc/gsr-helper/internal/gh"
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
	case edit.KindLabels, edit.KindGroup:
		return m.fetchFor(it.kind)
	case edit.KindEnv, edit.KindPath, edit.KindDropIn, edit.KindCopy:
		return m.openForm(it.kind)
	case edit.KindReregister:
		m.notice = "変更には再登録が必要です（Setup タブで削除して追加し直してください）"
		return nil
	default:
		return nil
	}
}

// fetchFor は GitHub 側の現在値を取ってからフォームを開く。一覧の組み立てでは
// API を呼ばず、項目を選んだこの時点で呼ぶ（3 秒ポーリングでは呼ばない方針）。
//
// **取得を始めた時点の対象を Msg に載せる。** 通信の最中に esc で別の runner へ
// 移れるため、載せずに戻ってくると A から取った値を B のフォームへ入れ、
// そのまま確定すれば B へ書き込まれる。
func (m *Model) fetchFor(k edit.Kind) tea.Cmd {
	m.busy = true
	in := m.commitInputOf(edit.Change{})
	dir := m.target.Dir

	return page.Do(m.tab, func() tea.Msg {
		ctx := context.Background()
		if k == edit.KindLabels {
			labels, err := edit.FetchLabels(ctx, in)
			return loadedMsg{kind: k, dir: dir, labels: labels, groups: nil, err: err}
		}

		groups, err := edit.FetchGroups(ctx, in)
		return loadedMsg{kind: k, dir: dir, labels: nil, groups: groups, err: err}
	})
}

// onLoaded は取得した現在値をフォームへ入れて開く。
func (m *Model) onLoaded(msg loadedMsg) tea.Cmd {
	m.busy = false

	// 取得中に対象が変わっていたら捨てる（fetchFor の doc）。
	if msg.dir != m.target.Dir {
		return nil
	}
	if msg.err != nil {
		m.notice = msg.err.Error()
		return nil
	}

	if msg.kind == edit.KindLabels {
		// 予約ラベル（self-hosted / Linux / X64）を落としてから初期値にする。
		// 落とさないと、開いた時点でフォーム自身の検証に落ちて確定できない。
		m.vals.LabelsBefore = config.CustomLabels(msg.labels)
		m.vals.Labels = edit.JoinLabels(m.vals.LabelsBefore)

		return m.openForm(edit.KindLabels)
	}
	return m.openGroupForm(msg.groups)
}

// openGroupForm は取得した runner group の一覧でフォームを開く。
//
// 一覧が空なら開かない。以前は名前を直接入力させていたが、ID を引けない値は
// 必ず ErrUnknownGroup で弾かれるため、入力させるだけの行き止まりだった。
func (m *Model) openGroupForm(groups []gh.RunnerGroup) tea.Cmd {
	if len(groups) == 0 {
		m.notice = "runner group の一覧を取得できませんでした"
		return nil
	}

	names := make([]string, 0, len(groups))
	ids := make([]int64, 0, len(groups))
	for _, g := range groups {
		names = append(names, g.Name)
		ids = append(ids, g.ID)
	}
	m.vals.SetGroups(names, ids)

	return m.openForm(edit.KindGroup)
}

// openForm は種類に応じてフォームの初期値を入れて開く。
func (m *Model) openForm(k edit.Kind) tea.Cmd {
	if err := m.vals.Fill(k, m.ld, m.target, m.others()); err != nil {
		m.notice = err.Error()
		return nil
	}

	m.formShown = true

	return m.overlay.Open(formKind, formOpenMsg{title: k.FormTitle(), values: m.vals, st: m.st})
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

	c, err := m.vals.Build(m.ld, m.target, m.others())
	if err != nil {
		m.notice = err.Error()
		m.overlay.Close()

		return nil
	}
	return m.approve(c)
}

// onApproved は差分の承認を処理する。承認されたときだけ書き込む。
//
// **決定は 1 度しか受けない**（Model.pendingSet の doc）。承認待ちの変更が無い
// 決定と、書き込みが走っている最中の決定は捨てる。
func (m *Model) onApproved(msg tea.Msg) tea.Cmd {
	d, ok := msg.(dialog.DecidedMsg)
	m.overlay.Close()

	if !m.pendingSet || m.busy {
		return nil
	}
	m.pendingSet = false

	if !ok || !d.Confirmed {
		m.pending, m.pendingSelf = edit.Change{}, appconfig.Config{}
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
		if err := edit.Commit(context.Background(), in); err != nil {
			return doneMsg{text: "", err: err}
		}
		return doneMsg{text: "書き込みました", err: nil}
	})
}
