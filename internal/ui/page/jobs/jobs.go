// Package jobs は Jobs タブを実装する。
//
// このホストで実行中のジョブを runner 横断で一覧する（FR-04 の表示形態）。
// **操作対象はジョブではなく runner である**（FR-47）。取り違えを防ぐため、フッタでも
// 対象が runner であることを明示する。
//
// 検出は自分で行わず、親 Model から page.StateMsg で受け取ったスナップショットを
// 描画に使う（atomic-design.md の page の責務）。
package jobs

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/logs"
	"github.com/ousiassllc/gsr-helper/internal/runner"
	"github.com/ousiassllc/gsr-helper/internal/ui/atom"
	"github.com/ousiassllc/gsr-helper/internal/ui/organism/table"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/action"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/runnerdetail"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/runnerop"
)

const (
	// inputFilter は入力中であることを状態行に出すときの名称（screens.md の入力中）。
	inputFilter = "絞り込み"
	// emptyMessage は実行中ジョブが無いときの表示（screens.md の Jobs タブ）。
	emptyMessage = "実行中のジョブはありません"
	// detailDesc は enter の説明（screens.md の Jobs タブのフッタ）。
	detailDesc = "runner の詳細"
)

// Model は Jobs タブ。
type Model struct {
	tab     int
	st      page.StateMsg
	tbl     table.Model[row]
	overlay page.Overlay
	// actions はキー定義から 1 度だけ組んだ操作の表。描画のたびに組み直さない
	// （action.Set の doc）。
	actions action.Set
	// ops は runner のサービス制御の制御部。Runners タブと同じものを持つ
	// （page/runnerop）。操作対象がジョブではなく runner であり（FR-47）、
	// 起点が違っても確認と実行の経路を 1 つに保つためである。
	ops runnerop.Model
	// initCmd はモーダルを登録したときに返った Cmd。最初の共有状態で流し、nil に落とす。
	initCmd tea.Cmd
	// info は Worker ログの解析結果、asked は解析を発行済みのジョブ（Issue #68）。
	// どちらも鍵は runner ディレクトリと Worker の PID の組（repo.go の jobKey）。
	info  map[string]logs.JobInfo
	asked map[string]struct{}
	// tries はジョブごとの解析の試行回数。ログが書かれる前に引いた場合の引き直しに
	// 上限を置くために持つ（repo.go の maxParseTries）。
	tries map[string]int
}

// tea.Model を実装していることをコンパイル時に確かめる。
var _ tea.Model = Model{}

// Init は何も発行しない。親はタブの Init を呼ばないため、登録が返した Cmd は
// 最初の page.StateMsg で流す（runners.go の Init と同じ理由）。
func (m Model) Init() tea.Cmd { return nil }

// Update は共有状態の反映とキー入力の解釈を行う。
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case page.StateMsg:
		return m.setState(msg)
	case tea.KeyPressMsg:
		return m.handleKey(msg)
	case jobInfoMsg:
		// Worker ログの解析結果（Issue #68）。REPOSITORY / `_work` 列に反映する。
		m.onJobInfo(msg)
		return m, m.chrome()
	case page.ResultMsg:
		// モーダルが返した決定は page が受ける（runners.go と同じ理由）。
		// page.Overlay.Handles が ResultMsg に偽を返すことと合わせた二重の守りで
		// あり、並びは関係しない（型スイッチの default は常に最後に評価される）。
		//
		// ログを開く決定だけは Logs タブへの移動なので handleResult が拾い、残りは
		// runnerop へ渡す。Runners タブと同じ確認フローを通すためであり、ここに
		// 独自の分岐を書くと Jobs タブだけ確認が変わりうる（FR-45〜FR-47）。
		return m.handleResult(msg)
	case runnerop.Msg:
		// 制御部宛の Msg はタブが受けて渡す（runners.go の handleOps と同じ理由。
		// 包まないと待機画面に吸われる）。Jobs タブは一括選択を持たないので、
		// 完了時に解く選択も無い。
		//
		// ops の呼び出しを return より前に出すのは、同じ tea.Batch に並べると
		// chrome が ops の変更前の状態を読み、DoneMsg の結果が ChromeMsg に載らず
		// 状態行が次の共有状態まで空のままになるためである。
		c := m.ops.Update(msg)
		return m, tea.Batch(m.chrome(), c)
	default:
		return m.forward(msg)
	}
}

// View はモーダルが開いていればそれを、無ければ一覧を返す。
func (m Model) View() tea.View {
	if m.overlay.Active() {
		return tea.NewView(m.overlay.View())
	}
	body := m.tbl.View()
	if body == "" {
		body = m.st.Styles.Muted.Render(emptyMessage)
	}
	return tea.NewView(body)
}

// setState は共有状態のスナップショットを反映する。ドメイン層は呼ばない。
func (m Model) setState(st page.StateMsg) (tea.Model, tea.Cmd) {
	m.st = st
	m.actions = action.NewSet(st.Keys.Runner, st.Scopes)
	// 配色を配り直すのは runners.go と同じ理由（table.Model.Restyle の doc）。
	m.tbl.Restyle(st.Keys.List, st.Styles)
	m.tbl.SetSize(st.BodyW, st.BodyH)
	m.tbl.SetItems(sectionJobs, jobRows(st.Result.Runners, m.info))
	// まだ引いていないジョブの Worker ログを解析する（Issue #68）。
	parse := m.resolveInfo(st.Result.Runners)
	m.ops.SetState(st, m.actions)
	// 登録の Cmd は return より前に取り出す（runners.go と同じ理由。同じ return 文に
	// 置くと、返り値 m の読み取りと m.initCmd の破棄の評価順が未規定になる）。
	init := m.flushInit()
	// モーダルが返す Cmd も親へ渡す（runners.go と同じ理由）。
	cmd := m.overlay.SetState(st)
	return m, tea.Batch(m.chrome(), init, cmd, parse)
}

// flushInit は登録が返した Cmd を 1 度だけ返す（runners.go と同じ理由）。
func (m *Model) flushInit() tea.Cmd {
	cmd := m.initCmd
	m.initCmd = nil
	return cmd
}

// forward はキー以外の Msg を配る。宛先の判定は page.Overlay.Handles に任せる
// （runners.go と同じ理由）。
func (m Model) forward(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	if m.overlay.Handles(msg) {
		m.overlay, cmd = m.overlay.Update(msg)
		return m, tea.Batch(m.chrome(), cmd)
	}
	m.tbl, cmd = m.tbl.Update(msg)
	return m, tea.Batch(m.chrome(), cmd)
}

// handleKey はキー入力を解釈する。
//
// 入力中とモーダル表示中は**親へ差し戻さない**。グローバルキーを閉じ込められるのは
// この判定を持つ page だけである（page.GlobalKeyMsg の doc）。
func (m Model) handleKey(press tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch {
	case m.tbl.Filtering(), m.overlay.Active():
		return m.forward(press)
	case key.Matches(press, m.st.Keys.Global.Help):
		cmd = m.overlay.OpenHelp()
	case key.Matches(press, m.st.Keys.List.Enter):
		cmd = m.openDetail()
	case key.Matches(press, m.st.Keys.Runner.Logs):
		cmd = m.openLogs()
	case key.Matches(press, m.st.Keys.Global.Back):
		// 直近の操作の結果も消す（runners.go の back と同じ理由）。
		m.ops.ClearStatus()
		m.tbl.ClearFilter()
	default:
		// runner の操作キー（d / X / R）はここで解釈する（FR-47）。対象はジョブでは
		// なく、カーソル位置のジョブを実行している runner 1 台である。**解釈した
		// キーは一覧へも親へも渡さない**（runners.go と同じ理由）。
		if c, ok := m.ops.HandleKey(press, runnerop.JobsOps(), m.targets()); ok {
			cmd = c
			break
		}
		// 自分が解釈しないキーは一覧へ渡し、同時に親へ差し戻す。タブ切替・再読み込み・
		// 終了を解釈するのは親であり、一覧のキーと衝突しないことは keymap の
		// 重複検査（keymap.Set.Contexts の「一覧画面（通常モード）」）が担保する。
		// **この経路が二重解釈の起きる場所である。** 自前のキー集合を Set に足す
		// タブは、そのキーが同時に有効になるコンテキストを Contexts へ登録すること
		// （登録漏れは TestContextsCoverEverySetField が落とす）。
		next, c := m.forward(press)
		return next, tea.Batch(c, page.BubbleKey(press))
	}
	return m, tea.Batch(m.chrome(), cmd)
}

// openDetail はカーソル位置のジョブを実行している runner の詳細画面を開く（FR-47）。
//
// ジョブ単体を止める機能は持たない。ホスト側からは Runner.Worker を強制終了する
// しかなく、ジョブが失敗として記録されるためである（screens.md の Jobs タブ）。
func (m *Model) openDetail() tea.Cmd {
	cur, ok := m.tbl.Selected()
	if !ok {
		return nil
	}
	return runnerdetail.Open(&m.overlay, cur.runner, m.st.Caps)
}

// targets は操作の対象を返す。
//
// 一括選択は使わない。Jobs タブの一覧は選択できず（rows.go の Selectable）、
// 対象はカーソル位置のジョブを実行している runner 1 台だからである。選び方そのものは
// Runners タブと同じ runnerop.Targets に通し、片方だけが別の決め方をしないようにする。
func (m Model) targets() []runner.Runner {
	cur, ok := m.tbl.Selected()
	return runnerop.Targets(nil, cur.runner, ok)
}

// chrome は親へ本体以外の状態を知らせる Cmd を返す。
func (m Model) chrome() tea.Cmd {
	c := page.ChromeMsg{
		Tab:    m.tab,
		Modal:  m.overlay.Active(),
		Input:  m.input(),
		Status: m.status(),
		Footer: m.footer(),
	}
	return func() tea.Msg { return c }
}

// input は入力中の名称を返す。入力中でなければ空文字を返す。
func (m Model) input() string {
	if m.tbl.Filtering() {
		return inputFilter
	}
	return ""
}

// status は状態行に出す page 側の文を返す。
//
// 選択件数は出さない。Jobs タブは複数選択して一括操作する画面ではなく（操作対象は
// カーソル位置のジョブを実行している runner 1 台）、区画も選択できないためである。
//
// 優先順は 入力中 → 直近の操作の結果。入力中はグローバルキーが効かない状態そのもの
// なので最優先で示す（runners.go の status と同じ規則。選択件数が無いぶん段が 1 つ少ない）。
func (m Model) status() string {
	if in := m.input(); in != "" {
		return "入力中: " + in
	}
	return m.ops.Status()
}

// footer はフッタのキーヒントを返す。
//
// 出す操作と表記は keymap.RunnerKeys.JobsFooter に従い、Jobs タブ側では持たない。
// キーと説明文の出どころを 1 つにするためである（page が文言を書き直すと、キーを
// 差し替えたときにこのフッタだけが古くなる）。可否と理由の判定も page.Allowed に
// 任せ、同じ操作の理由が Runners タブと食い違わないようにする。
//
// 先頭の `enter:runner の詳細` は、操作対象がジョブではなく runner であることの明示を
// 兼ねる（FR-47）。全キーに「runner を」を付けると幅 80 に収まらず、有効なキーが
// フッタから落ちる（設計原則 1 に反する）。
func (m Model) footer() []atom.Hint {
	if m.overlay.Active() {
		return m.overlay.Hints()
	}
	cur, ok := m.tbl.Selected()
	if !ok {
		return nil
	}

	keys := m.st.Keys.Runner
	footer := keys.JobsFooter()
	hints := make([]atom.Hint, 0, len(footer)+1)
	hints = append(hints, atom.Hint{
		Key:     page.BindingKey(m.st.Keys.List.Enter),
		Desc:    detailDesc,
		Enabled: true,
		Reason:  "",
	})
	for _, f := range footer {
		k := page.BindingKey(f.Binding)
		enabled, reason := m.actions.Allowed(k, cur.runner, m.st.Caps)
		hints = append(hints, atom.Hint{
			Key:     k,
			Desc:    f.Desc,
			Enabled: enabled,
			Reason:  reason,
		})
	}
	return hints
}
