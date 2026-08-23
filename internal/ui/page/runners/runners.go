// Package runners は Runners タブ（既定画面）を実装する。
//
// ホスト上の runner 一覧と孤児ユニットを 2 区画で表示し、enter で詳細画面を開く。
// 検出は自分で行わず、親 Model から page.StateMsg で受け取ったスナップショットを
// 描画に使う（atomic-design.md の page の責務）。
package runners

import (
	"strconv"

	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/ui/atom"
	"github.com/ousiassllc/gsr-helper/internal/ui/organism/table"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/action"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/runnerdetail"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/runnerop"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/runners/rowview"
)

const (
	// inputFilter は入力中であることを状態行に出すときの名称（screens.md の入力中）。
	inputFilter = "絞り込み"
	// emptyMessage は runner が 1 台も見つからないときの表示。
	emptyMessage = "runner が見つかりません（--root で走査ルートを追加できます）"
)

// Model は Runners タブ。
type Model struct {
	tab     int
	st      page.StateMsg
	tbl     table.Model[rowview.Row]
	overlay page.Overlay
	// actions はキー定義から 1 度だけ組んだ操作の表。描画のたびに組み直さない
	// （action.Set の doc）。
	actions action.Set
	// ops は runner のサービス制御の制御部。確認・実行・結果の報告を持つ。
	//
	// Jobs タブと同じものを持つ（page/runnerop）。起点が違っても確認と実行の
	// 経路を 1 つに保つためである（screens.md の設計原則 6）。
	ops runnerop.Model
	// initCmd はモーダルを登録したときに返った Cmd。最初の共有状態で流し、nil に落とす。
	initCmd tea.Cmd
	// notice は Setup タブへ移せなかった理由。次の打鍵で消える。
	//
	// ops.Status とは別に持つ。サービス制御の結果報告とは寿命も出どころも違い、
	// 同じ入れ物に混ぜると片方が他方を黙って上書きする。
	notice string
}

// tea.Model を実装していることをコンパイル時に確かめる。
var _ tea.Model = Model{}

// New は Runners タブを組み立てる。tab は親が持つタブ番号で、ChromeMsg に載せる。
//
// モーダルは画面が登録する。重なりの規則（キーは最上位だけ・esc は 1 枚）は
// page.Overlay が種類に依らず担保するので、タブを足す Issue は自分のモーダルを
// Register するだけで済む。
func New(tab int, st page.StateMsg) Model {
	// ヘルプ（NewOverlay が登録する）と詳細画面、どちらの登録が返した Cmd も畳み込む。
	// 片方でも捨てると「登録が返す Cmd は呼び出し側まで返す」規則を構築時に破る
	// （page.Overlay.Register の doc）。
	overlay, help := page.NewOverlay(tab, st)
	detail := overlay.Register(runnerdetail.Kind, runnerdetail.New(st))
	// 確認ダイアログと待機画面の登録は runnerop が行う（種類の綴りをタブ側が
	// 知らずに済む）。ここでもその Cmd を畳み込む。
	ops, opsCmd := runnerop.New(tab, overlay, st)
	cmd := tea.Batch(help, detail, opsCmd)
	return Model{
		tab:     tab,
		st:      st,
		tbl:     rowview.NewTable(st.Keys, st.Styles),
		overlay: overlay,
		actions: action.NewSet(st.Keys.Runner, st.Scopes),
		ops:     ops,
		initCmd: cmd,
		notice:  "",
	}
}

// Init は何も発行しない。
//
// **親はタブの Init を呼ばない。** bubbletea が Init を呼ぶのはルート Model
// （ui.App）だけであり、App.Init は自分の Cmd しか返さない。ここで initCmd を
// 返してもランタイムには届かず、登録した時点で処理を始めるモーダルは動かない。
// そこで登録が返した Cmd は最初の page.StateMsg で流す（setState）。共有状態は親が
// 必ず全有効タブへ配るため、この経路なら確実に届く。
//
// 発行点を片方に寄せるのは二重発火を避けるためである。両方で返すと、親が Init を
// 呼ぶようになった時点で登録の Cmd が 2 度実行される。
//
// 検出は親が駆動するため自分では発行しない。
func (m Model) Init() tea.Cmd { return nil }

// Update は共有状態の反映とキー入力の解釈を行う。
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case page.StateMsg:
		return m.setState(msg)
	case tea.KeyPressMsg:
		return m.handleKey(msg)
	case page.ResultMsg:
		// モーダルが返した決定は page が受ける。転送すると決定は発行元のモーダル
		// 自身へ戻り、そこで捨てられる（page.ResultMsg の doc）。
		//
		// **page.Overlay.Handles が ResultMsg に偽を返すことと合わせた二重の守り
		// である。** 決定を解釈するのは Overlay ではなく page だという分担を、この
		// case が page 側から明示する（Go の型スイッチの default は記述位置に
		// 関わらず最後に評価されるため、並びは関係しない）。
		//
		// ログを開く決定だけは Logs タブへの移動なので handleResult が拾い、残りは
		// runnerop が解釈する（詳細画面・確認ダイアログ・待機画面のいずれの決定も）。
		// Jobs タブと同じ経路にすることで、起点によって確認の強さが変わらない。
		return m.handleResult(msg)
	case runnerop.Msg:
		return m.handleOps(msg)
	default:
		return m.forward(msg)
	}
}

// handleOps は runner 操作の制御部が自分宛に発行した Msg を処理する。
//
// **タブが自分の case で受ける。** 包まれていない Msg は forward で Overlay へ渡り
// （page.Overlay.Handles はモーダルを開いている間の既定で真を返す）、待機画面に
// 吸われて制御部へ届かない。ドレイン停止は待機画面を開いたまま結果を待つので、
// 受け損ねると待機が永久に終わらない。
func (m Model) handleOps(msg runnerop.Msg) (tea.Model, tea.Cmd) {
	if _, done := msg.Payload.(runnerop.DoneMsg); done {
		// 実行し終えた対象の選択は解く。残したままだと、同じ集合へ二度目の
		// 一括操作を打ててしまう（メンテナンス前の全停止の直後に別のキーを
		// 打つ経路で起きる）。選び直させるほうが安全側である。
		m.tbl.ClearSelection()
	}
	// ops の呼び出しは return より前に出す（case page.ResultMsg と同じ理由）。
	// **同じ return 文に置いてはならない。** 並べると chrome が m.ops.Update より
	// 先に評価され、DoneMsg の結果文字列（m.ops.Status()）が ChromeMsg に載らない。
	// 直前に選択を解いているので「選択: N 件」も消えており、状態行は次の共有状態
	// （既定 3 秒後）まで丸ごと空白になる。一括操作の成否を読む手がかりが消える。
	c := m.ops.Update(msg)
	return m, tea.Batch(m.chrome(), c)
}

// View はモーダルが開いていればそれを、無ければ一覧を返す。
//
// 本体に重ねる合成はしない（template.Modal の方式に合わせる）。
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

// setState は共有状態のスナップショットを反映する。
//
// ドメイン層は呼ばない。返すのは親へ本体以外の状態を知らせる ChromeMsg の Cmd だけで、
// 検出は親が 1 本の Cmd で駆動する（同じ検出が重複実行されないようにするため）。
func (m Model) setState(st page.StateMsg) (tea.Model, tea.Cmd) {
	m.st = st
	m.actions = action.NewSet(st.Keys.Runner, st.Scopes)
	// 背景色は起動後に届き、切り替わることもある。配色を配り直さないと一覧の中身だけが
	// 古い明暗のまま残る（table.Model.Restyle の doc）。
	m.tbl.Restyle(st.Keys.List, st.Styles)
	m.tbl.SetSize(st.BodyW, st.BodyH)
	m.tbl.SetItems(rowview.SectionRunners, rowview.Runners(st.Result.Runners, st.Disk))
	m.tbl.SetItems(rowview.SectionOrphans, rowview.Orphans(st.Result.OrphanUnits))
	m.ops.SetState(st, m.actions)
	// 登録の Cmd は return より前に取り出す。**同じ return 文に置いてはならない。**
	// 返り値の m（非関数オペランド）の読み取りと m.flushInit() による m.initCmd の
	// 破棄は、Go 仕様では評価順が未規定であり、「2 度目からは nil」という flushInit の
	// 約束が言語仕様の側から保証されなくなる。
	init := m.flushInit()
	// モーダルが返す Cmd も親へ渡す（開いているモーダルが共有状態を受けて
	// 何かを始めることがある。捨てるとその処理が動かない）。
	cmd := m.overlay.SetState(st)
	return m, tea.Batch(m.chrome(), init, cmd)
}

// flushInit は登録が返した Cmd を 1 度だけ返す。2 度目からは nil を返す。
//
// 親がタブの Init を呼ばない以上、ここが登録の Cmd をランタイムへ渡す唯一の経路で
// ある（Init の doc）。nil に落とすのは、共有状態が 3 秒ごとに届くため、落とさないと
// 登録時の処理が周期ごとに再実行されるためである。
func (m *Model) flushInit() tea.Cmd {
	cmd := m.initCmd
	m.initCmd = nil
	return cmd
}

// forward はキー以外の Msg を配る。
//
// 宛先が Overlay かどうかの判定は page.Overlay.Handles に任せる。開閉だけで
// 判断すると、宛先を明示した page.ModalMsg が閉じている間に捨てられる。
// Overlay が受けないものは一覧へ渡す（絞り込みのカーソル点滅など、
// organism/table が発行を続ける Cmd を止めないため）。
func (m Model) forward(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	if m.overlay.Handles(msg) {
		m.overlay, cmd = m.overlay.Update(msg)
		return m, tea.Batch(m.chrome(), cmd)
	}
	m.tbl, cmd = m.tbl.Update(msg)
	return m, tea.Batch(m.chrome(), cmd)
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
// 孤児ユニット件数と警告件数は親が出す（検出結果を持っているのは親であり、
// タブが変わっても同じ件数を出すため）。ここでは page しか知らない入力中・
// 選択件数・直近の操作の結果を返す。
//
// **優先順は 入力中 → 選択件数 → 操作の結果 である。**
//
//   - 入力中はグローバルキーが効かない状態そのものなので最優先で示す。示さないと
//     画面が無反応になったように見える（screens.md の入力中）。
//   - 移動できなかった理由（notice）は選択件数より先に出す。直前の打鍵に対する
//     応答であり、出さないと n / D / u が無反応に見える。
//   - 選択件数を結果より先に出すのは、実行を終えた時点で選択を解く（handleOps）ため、
//     両方が同時に非空になるのは「結果を見た後に次の対象を選び始めた」場面に限られる
//     からである。そこで必要なのは、済んだ操作の報告ではなく今から何台に効くかである。
//   - 結果は次の操作か esc（back）まで残す。3 秒ごとの再検出で消えると、一括操作の
//     失敗した runner 名を読み終える前に流れる。
func (m Model) status() string {
	if in := m.input(); in != "" {
		return "入力中: " + in
	}
	if m.notice != "" {
		return m.notice
	}
	if n := len(m.tbl.Checked()); n > 0 {
		return "選択: " + strconv.Itoa(n) + " 件"
	}
	return m.ops.Status()
}

// footer はフッタのキーヒントを返す。
//
// モーダル表示中はそのモーダルのキー、孤児ユニットの区画では一覧の移動キー、
// runner の行ではカーソル位置の runner に対する操作の可否を出す。
func (m Model) footer() []atom.Hint {
	if m.overlay.Active() {
		return m.overlay.Hints()
	}
	cur, ok := m.tbl.Selected()
	if !ok || cur.IsOrphan {
		return m.listHints()
	}
	return m.actions.Hints(cur.Runner, m.st.Caps, m.st.Keys.Runner)
}

// listHints は操作の対象が無いときのフッタを返す。
//
// 孤児ユニットに対する操作はこの版に無く、enter でも詳細を開けないため、一覧の
// 移動と絞り込みだけを出す。キーと説明は keymap から取り、フッタとヘルプで文言が
// 食い違わないようにする。
//
// キー表記は page.BindingKey で作る。Help().Key は "j/↓" のように別名を含む表記で、
// 詳細画面のフッタ（page.RunnerDetail.Hints）は "j/k" なので、同じフッタの中で
// 移動キーの表記が 2 通りになるためである。
func (m Model) listHints() []atom.Hint {
	l := m.st.Keys.List
	return []atom.Hint{
		{
			Key:     page.BindingKey(l.Down) + "/" + page.BindingKey(l.Up),
			Desc:    "移動",
			Enabled: true,
			Reason:  "",
		},
		{Key: page.BindingKey(l.Filter), Desc: l.Filter.Help().Desc, Enabled: true, Reason: ""},
	}
}
