// Package disk は Disk タブを実装する。
//
// runner のディスク使用量を内訳ごとに一覧し、選んだ対象をクリーンアップする
// （FR-27〜FR-31）。**このタブだけが破壊的操作を持つ**ため、削除に至る道は
// 「選択 → ドライラン → 確認 → 実行」の 1 本に絞ってある（clean.go）。
//
// 検出結果（runner の一覧）は自分で取らず、親 Model から page.StateMsg で受け取った
// スナップショットを使う（atomic-design.md の page の責務）。一方で**使用量の集計は
// 自分で駆動する。** 親が持つ共有状態は 3 秒ごとの再検出で全タブへ配られるものであり、
// 数分かかりうるディスク走査をそこに載せると全タブが道連れになる。集計はタブの寿命
// （page.ActivateMsg / DeactivateMsg / ShutdownMsg）に紐付けて張り、畳む。
//
// 同名の internal/disk を import している。パッケージ自身の名前は識別子として
// スコープに入らないため衝突しない。別名を付けないのは、ドメインの型が disk.Usage /
// disk.CleanPlan として読めることに価値があるためである。
package disk

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/disk"
	"github.com/ousiassllc/gsr-helper/internal/ui/keymap"
	"github.com/ousiassllc/gsr-helper/internal/ui/molecule"
	"github.com/ousiassllc/gsr-helper/internal/ui/organism/table"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/disk/confirmmodal"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/progressmodal"
)

// Model は Disk タブ。
type Model struct {
	tab     int
	st      page.StateMsg
	tbl     table.Model[row]
	overlay page.Overlay
	// initCmd はモーダルを登録したときに返った Cmd。最初の共有状態で流し、nil に落とす。
	initCmd tea.Cmd

	// stats / statsErr はファイルシステムの容量と inode（FR-29）。
	stats    disk.Stats
	statsErr error

	// gen は集計の世代。古い集計の結果を捨てるために持つ（scan.go の usageMsg）。
	gen int
	// seen は判明順に積んだ集計結果。表の行はここから作る。
	//
	// **集計より寿命が長い。** 裏へ回って集計を畳んでも行と選択は捨てないため
	// （page.DeactivateMsg の doc）、実行中の集計（scan）ではなく Model が持つ。
	seen []disk.Usage

	// scan は実行中の集計。nil なら集計していない。
	scan *scanState
	// clean は実行中のクリーンアップ。nil なら実行していない。
	clean *cleanState
	// plan は確認中のドライラン結果。確認を閉じた時点で捨てる。
	plan disk.CleanPlan
	// notice は結果報告と操作できない理由。次の打鍵で消す。
	notice string
}

// tea.Model を実装していることをコンパイル時に確かめる。
var _ tea.Model = Model{}

// New は Disk タブを組み立てる。tab は親が持つタブ番号で、ChromeMsg に載せる。
//
// 登録（ヘルプ・確認ダイアログ）と ? の範囲の宣言が返した Cmd はすべて畳み込む。
// 1 つでも捨てると「登録が返す Cmd は呼び出し側まで返す」規則を構築時に破る
// （page.Overlay.Register の doc）。
func New(tab int, st page.StateMsg) Model {
	overlay, help := page.NewOverlay(tab, st)
	cf := overlay.Register(confirmmodal.Kind, confirmmodal.New(st))
	// クリーンアップの逐次表示と結果報告（FR-15 / Issue #75）。確認と別のモーダルに
	// するのは、承認のあとに開くもので、実行中は esc を握って閉じさせないためである
	// （progressmodal.handlesBack）。
	pr := overlay.Register(progressmodal.Kind, progressmodal.New(st))
	// ? に出すキーの範囲は自分で宣言する（atomic-design.md の約束 6）。既定は
	// runner を並べる一覧向けであり、このタブでは効かない runner 操作キーが並ぶ。
	scope := overlay.SetHelpScope(keymap.Set.DiskHelp)
	return Model{
		tab:      tab,
		st:       st,
		tbl:      newTable(st.Keys, st.Styles),
		overlay:  overlay,
		initCmd:  tea.Batch(help, cf, pr, scope),
		stats:    disk.Stats{},
		statsErr: nil,
		gen:      0,
		seen:     nil,
		scan:     nil,
		clean:    nil,
		plan:     emptyPlan(),
		notice:   "",
	}
}

// Init は何も発行しない。
//
// **親はタブの Init を呼ばない**ため、登録が返した Cmd は最初の page.StateMsg で流す
// （runners.go の Init と同じ理由）。集計もここでは始めない。Disk は既定タブではなく、
// 前面に出た時点で page.ActivateMsg が必ず届くためである（page.ActivateMsg の doc）。
func (m Model) Init() tea.Cmd { return nil }

// Update は共有状態・キー・寿命の通知・集計と削除の結果を振り分ける。
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case page.StateMsg:
		return m.setState(msg)
	case tea.KeyPressMsg:
		return m.handleKey(msg)
	case page.ResultMsg:
		// モーダルが返した決定は page が受ける。転送すると決定は発行元のモーダル
		// 自身へ戻り、そこで捨てられる（page.ResultMsg の doc）。
		// page.Overlay.Handles が ResultMsg に偽を返すことと合わせた二重の守りで
		// ある（型スイッチの default は記述位置に関わらず最後に評価されるため、
		// 並びは関係しない）。
		cmd := m.onResult(msg)
		return m, tea.Batch(m.chrome(), cmd)
	case page.ActivateMsg:
		cmd := m.startScan()
		return m, tea.Batch(m.chrome(), cmd)
	case page.DeactivateMsg:
		m.stopScan()
		return m, m.chrome()
	case page.ShutdownMsg:
		m.stopScan()
		m.stopClean()
		return m, m.chrome()
	case usageMsg:
		cmd := m.onUsage(msg)
		return m, tea.Batch(m.chrome(), cmd)
	case fsStatsMsg:
		m.onFSStats(msg)
		return m, m.chrome()
	case progressMsg:
		cmd := m.onProgress(msg)
		return m, tea.Batch(m.chrome(), cmd)
	case applyDoneMsg:
		cmd := m.onApplyDone(msg)
		return m, tea.Batch(m.chrome(), cmd)
	default:
		next, cmd := m.forwardTo(msg)
		return next, cmd
	}
}

// View はモーダルが開いていればそれを、無ければ要約行と一覧を返す。
func (m Model) View() tea.View {
	if m.overlay.Active() {
		return tea.NewView(m.overlay.View())
	}

	body := m.tbl.View()
	if body == "" {
		body = m.st.Styles.Muted.Render(m.emptyMessage())
	}
	summary := molecule.FSSummaryLine(m.summaryView(), m.st.BodyW, m.st.Styles)
	return tea.NewView(summary + "\n\n" + body)
}

// setState は共有状態のスナップショットを反映する。
//
// 順序は runners.go に合わせる（配色 → 大きさ → 行 → 登録の Cmd → モーダルへの配布）。
// 集計はここから始めない。共有状態は 3 秒ごとに届くため、始めると走査が積み上がる。
func (m Model) setState(st page.StateMsg) (tea.Model, tea.Cmd) {
	m.st = st
	// 背景色は起動後に届き、切り替わることもある。配り直さないと一覧の中身だけが
	// 古い明暗のまま残る（table.Model.Restyle の doc）。
	m.tbl.Restyle(st.Keys.List, st.Styles)
	m.tbl.SetSize(st.BodyW, tableHeight(st.BodyH))
	m.tbl.SetItems(sectionTargets, usageRows(m.seen))
	// 登録の Cmd は return より前に取り出す。**同じ return 文に置いてはならない。**
	// 返り値の m の読み取りと m.initCmd の破棄は Go 仕様では評価順が未規定であり、
	// 「2 度目からは nil」という flushInit の約束が言語仕様の側から保証されなくなる。
	init := m.flushInit()
	cmd := m.overlay.SetState(st)
	// キー定義が差し替わった場合に備えて範囲を宣言し直す。Overlay は範囲を関数で
	// 持つので、同じ範囲を送り直しても中身が作り直されるだけでスクロール位置は残る。
	scope := m.overlay.SetHelpScope(keymap.Set.DiskHelp)
	return m, tea.Batch(m.chrome(), init, cmd, scope)
}

// flushInit は登録が返した Cmd を 1 度だけ返す。2 度目からは nil を返す
// （runners.go と同じ理由）。
func (m *Model) flushInit() tea.Cmd {
	cmd := m.initCmd
	m.initCmd = nil
	return cmd
}

// forwardTo はキー以外の Msg を配る。宛先の判定は page.Overlay.Handles に任せる
// （runners.go と同じ理由）。
//
// 具体型のまま返すのは、配送の直後に自分の状態を触る呼び出し元（handleKey の
// 閉じ込め分岐）が型アサーションを挟まずに済むようにするためである。
func (m Model) forwardTo(msg tea.Msg) (Model, tea.Cmd) {
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
// この判定を持つ page だけであり（page.GlobalKeyMsg の doc）、ここで差し戻すと
// **削除の確認中に打った q でアプリが終わる。**
//
// space（選択のトグル）を自分で解釈しないのは、選択が organism/table のローカル状態
// だからである。選択できない行にチェックが付かないことも table が担保する。
func (m Model) handleKey(press tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	// 前の打鍵で出した報告は次の打鍵で消す（状態行に残り続けないようにする）。
	m.notice = ""

	var cmd tea.Cmd
	switch {
	case m.tbl.Filtering(), m.overlay.Active():
		next, c := m.forwardTo(press)
		next.dropAbandonedPlan()
		return next, c
	case key.Matches(press, m.st.Keys.Global.Help):
		cmd = m.overlay.OpenHelp()
	case key.Matches(press, m.st.Keys.Disk.Clean):
		cmd = m.requestClean()
	case key.Matches(press, m.st.Keys.Global.Refresh):
		// r は「再集計」であると同時に親の「再読み込み」でもある（screens.md の
		// Disk タブ）。自分で集計を張り直したうえで親へも差し戻すのは、二重解釈では
		// なく**両方が自分の再読み込みを行う**のが正しい振る舞いだからである。
		// 差し戻さないと、Disk タブに居る間だけ runner の再検出が止まる。
		cmd = m.startScan()
		return m, tea.Batch(m.chrome(), cmd, page.BubbleKey(press))
	case key.Matches(press, m.st.Keys.Global.Back):
		m.back()
	default:
		// 自分が解釈しないキーは一覧へ渡し、同時に親へ差し戻す。一覧のキーと
		// 衝突しないことは keymap の重複検査（Set.Contexts の「Disk タブ
		// （通常モード）」）が担保する。
		next, c := m.forwardTo(press)
		return next, tea.Batch(c, page.BubbleKey(press))
	}
	return m, tea.Batch(m.chrome(), cmd)
}

// dropAbandonedPlan は確認ダイアログが決定を出さずに閉じたときに計画を捨てる。
//
// esc は Overlay が 1 枚閉じる経路を通る（Modal.HandlesBack が nil）ため、
// dialog.Confirm は DecidedMsg を発行せず onResult も呼ばれない。捨てる場所が
// ここにしか無い。**残っていても実行はされない**（disk.Apply へ至るのは
// dialog.DecidedMsg{Confirmed: true} を受けた onResult だけである）が、利用者が
// 取り消した計画を抱えたままの状態を作らない。
func (m *Model) dropAbandonedPlan() {
	if m.overlay.Active() {
		return
	}
	m.plan = emptyPlan()
}

// back は esc の「選択のクリア / 1 つ前の状態へ戻る」を処理する。
//
// 選択を先に解くのは、選択したまま絞り込みを解除すると画面外の対象が選択されたまま
// 残るためである（screens.md のグローバルキー）。**このタブでは選択がそのまま削除
// 対象になる**ので、見えない選択が残ることの危険が他のタブより大きい。
func (m *Model) back() {
	if len(m.tbl.Checked()) > 0 {
		m.tbl.ClearSelection()
		return
	}
	m.tbl.ClearFilter()
}
