// Package config は Config タブ（対話型設定編集）を提供する。
//
// 画面は「対象の runner を選ぶ → 項目を選ぶ → フォーム → 差分の承認 → 書き込み →
// 反映方法の選択」と進む（docs/ui/screens.md の Config タブ、FR-35〜FR-40）。
//
// **差分に出す内容と実際に書き込む内容を同じ値から作る**（change の doc）。別々に
// 組むと、承認した内容と書かれる内容が食い違う余地ができる。
//
// **確認を経ない書き込み経路を作らない。** commit を呼ぶのは差分の承認
// （dialog.DecidedMsg{Confirmed: true}）を受けた onResult 1 か所だけである
// （page/setup・page/disk と同じ構造）。
//
// ラベルと runner group は GitHub 側の値なので internal/gh へ委ね、再起動を伴わない。
// ファイルを書き換える項目だけが反映方法の選択へ進む。
package config

import (
	"context"

	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/appconfig"
	"github.com/ousiassllc/gsr-helper/internal/exec"
	"github.com/ousiassllc/gsr-helper/internal/gh"
	"github.com/ousiassllc/gsr-helper/internal/runner"
	"github.com/ousiassllc/gsr-helper/internal/ui/keymap"
	"github.com/ousiassllc/gsr-helper/internal/ui/molecule/listrow"
	"github.com/ousiassllc/gsr-helper/internal/ui/organism"
	"github.com/ousiassllc/gsr-helper/internal/ui/organism/table"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
	"github.com/ousiassllc/gsr-helper/internal/ui/token"
)

// 画面の見出し。
const (
	titleApply  = "反映方法を選んでください"
	titlePicker = "設定を編集する runner を選んでください"
)

// Model は Config タブ。
type Model struct {
	tab     int
	st      page.StateMsg
	overlay page.Overlay
	// initCmd は modal の登録が返した Cmd。最初の StateMsg で親へ流す。
	initCmd tea.Cmd

	ld   loader
	list table.Model[item]
	// picker は対象の runner を選ぶ一覧。対象が決まるまで本文に出す。
	picker organism.ChoiceList
	// vals はフォームの入力先。huh がポインタで束縛するため実体を持ち続ける。
	vals *values

	target  runner.Runner
	pending change
	// pendingSelf は承認待ちの自身の設定（FR-41〜FR-42）。
	pendingSelf appconfig.Config
	// self は自身の設定（FR-41〜FR-42）を編集中か。
	self bool

	notice string
	report string
	// busy は書き込みや API 呼び出しが走っているか。
	busy bool
	// newClient は GitHub API のクライアントを作る。テストで差し替える。
	newClient func(ctx context.Context, ex exec.Executor) (*gh.Client, error)
	// formShown はフォームを表示中か。状態行の「入力中」の判定に使う。
	formShown bool
}

// tea.Model を実装していることをコンパイル時に確かめる。
var _ tea.Model = Model{}

// New は Config タブを組み立てる。
func New(tab int, st page.StateMsg) Model {
	overlay, help := page.NewOverlay(tab, st)
	form := overlay.Register(formKind, newFormModal(st))
	diff := overlay.Register(diffKind, newDiffModal(st))
	applyReg := overlay.Register(applyKind, newApplyModal(st))
	scopeCmd := overlay.SetHelpScope(keymap.Set.ConfigHelp)

	m := Model{
		tab: tab, st: st, overlay: overlay,
		initCmd: tea.Batch(help, form, diff, applyReg, scopeCmd),
		ld:      loader{dropInRoot: ""},
		list:    newList(st),
		picker:  organism.NewChoiceList(st.Keys.List, st.Styles),
		vals:    newValues(),
		target:  runner.Runner{}, pending: change{}, pendingSelf: appconfig.Config{}, self: false,
		notice: "", report: "", busy: false, formShown: false,
		newClient: defaultClient,
	}
	m.refresh(organism.ResetCursor)

	return m
}

// newList は設定項目の一覧を組み立てる。
//
// 区画を 3 つに分けるのは、画面仕様のモックが「編集できる項目」「再登録が要る
// 項目」「複製」を区切り線で分けているためである。
func newList(st page.StateMsg) table.Model[item] {
	sec := func(title string, selectable bool) table.SectionInput[item] {
		return table.SectionInput[item]{
			Title: title, Columns: settingColumns(), Rules: token.ColumnRules{},
			Render: func(in table.RowInput[item]) []string {
				return listrow.SettingRow(in.Item.view, in.Cols, in.Styles)
			},
			ID:         func(it item) string { return it.view.Item },
			Match:      matches,
			Disabled:   disabledReason,
			Selectable: selectable,
		}
	}

	return table.New(st.Keys.List, st.Styles,
		sec("", false),
		sec("再登録が必要な項目", false),
		sec("複製", false),
	)
}

// Init は何も発行しない。親はタブの Init を呼ばず、最初の StateMsg で駆動する。
func (m Model) Init() tea.Cmd { return nil }

// Update は Msg を種類ごとに振り分ける。
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case page.StateMsg:
		return m.setState(msg)
	case tea.KeyPressMsg:
		return m.handleKey(msg)
	case page.ResultMsg:
		cmd := m.onResult(msg)
		return m, tea.Batch(m.chrome(), cmd)
	case organism.ChosenMsg:
		cmd := m.onPicked(msg)
		return m, tea.Batch(m.chrome(), cmd)
	case page.EditConfigMsg:
		m.setTarget(msg.Runner)
		return m, m.chrome()
	case loadedMsg:
		cmd := m.onLoaded(msg)
		return m, tea.Batch(m.chrome(), cmd)
	case doneMsg:
		cmd := m.onDone(msg)
		return m, tea.Batch(m.chrome(), cmd)
	case page.ActivateMsg, page.DeactivateMsg, page.ShutdownMsg:
		return m, m.chrome()
	default:
		return m.forwardTo(msg)
	}
}

// setState は共有状態を受けて表示を組み直す。
func (m Model) setState(st page.StateMsg) (tea.Model, tea.Cmd) {
	first := m.st.Config.Path == "" && st.Config.Path != ""
	m.st = st
	m.list.Restyle(st.Keys.List, st.Styles)
	m.list.SetSize(st.BodyW, st.BodyH)
	m.picker.Restyle(st.Keys.List, st.Styles)
	m.picker.SetWidth(st.BodyW)
	m.refresh(organism.KeepCursor)

	// 登録の Cmd は return より前に取り出す（page/setup の setState と同じ）。
	init := m.flushInit()
	state := m.overlay.SetState(st)
	scopeCmd := m.overlay.SetHelpScope(keymap.Set.ConfigHelp)

	// 設定ファイルが無い状態で起動したら初回ウィザードを出す（FR-41）。
	var wizard tea.Cmd
	if first && st.Config.FirstRun && !m.self {
		wizard = m.openSelfForm()
	}

	return m, tea.Batch(m.chrome(), init, state, scopeCmd, wizard)
}

// flushInit は登録の Cmd を 1 度だけ返す。
func (m *Model) flushInit() tea.Cmd {
	cmd := m.initCmd
	m.initCmd = nil
	return cmd
}

// refresh は本文の一覧を組み直す。
func (m *Model) refresh(policy organism.CursorPolicy) {
	if m.target.Dir == "" {
		m.picker.SetItems(m.pickerItems(), policy)
		return
	}

	s := summarize(m.target, m.ld)
	m.list.SetItems(secMain, mainItems(s))
	m.list.SetItems(secReregister, reregisterItems())
	m.list.SetItems(secCopy, copyItems(len(m.others()) > 0))
}

// View はモーダルがあればそれを、無ければ本文を描く。
func (m Model) View() tea.View {
	if m.overlay.Active() {
		return tea.NewView(m.overlay.View())
	}
	if m.target.Dir == "" {
		return tea.NewView(m.st.Styles.Header.Render(titlePicker) + "\n\n" + m.picker.View())
	}

	head := m.st.Styles.Header.Render(m.target.Name() + " の設定")
	return tea.NewView(head + "\n\n" + m.list.View())
}

// matches は絞り込みの一致判定。項目名と現在値のどちらかに含まれれば残す。
func matches(it item, q string) bool {
	return containsFold(it.view.Item, q) || containsFold(it.view.Value, q)
}

// disabledReason は行を選択できない理由を返す。
//
// 理由は行の備考欄（SettingView.Note）が既に持っているため、ここでは空文字を
// 返して二重に出さない。返すのは可否だけである。
func disabledReason(it item) (string, bool) { return "", !it.enabled }
