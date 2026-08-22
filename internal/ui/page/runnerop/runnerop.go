// Package runnerop は Runners / Jobs タブが共用する runner のサービス制御を担う。
//
// **操作の起点は複数あるが、確認と実行の経路は 1 つである**（screens.md の設計原則 6
// 「操作の起点は複数、確認は 1 つ」）。一覧の直接キー・詳細画面の操作リスト・Jobs タブ
// （FR-45〜FR-47）のどこから起動しても、対象の決定・可否の再判定・確認ダイアログの
// 組み立て・実行・結果の報告はこのパッケージを通る。両タブへ書き写すと、起点によって
// 確認の強さが変わる余地（FR-45〜FR-47 が明示的に禁じたもの）が生まれ、片方のタブだけ
// 確認を飛ばす退行がコンパイルも検査も通ってしまう。
//
// タブ側に残るのは「どのキーを直接受けるか」と「どの行を対象に渡すか」だけである。
// この 2 つはタブごとに違う（Runners は 6 操作と一括選択、Jobs は 3 操作とカーソル 1 件）。
//
// **タブではない。** page/ 直下の共有部品なので page/pagetest/import_test.go の
// shared に登録してある（登録しないと TestOnlyTabsetImportsTabs がタブと誤認する）。
//
// ドメイン層（internal/svc）を tea.Cmd で呼ぶのは page 階層の役目であり
// （atomic-design.md の依存の規則）、このパッケージがその呼び出し点である。
package runnerop

import (
	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/runner"
	"github.com/ousiassllc/gsr-helper/internal/ui/organism/dialog"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/action"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/runnerdetail"
)

// Model は runner 操作の制御部。タブが 1 つ持ち、キー・共有状態・決定を配る。
//
// **page.Overlay を写しで持つ。** Overlay の写しは実体（*overlayState）を共有する
// （page.Overlay の doc）ため、ここで開いたモーダルはタブ側の overlay.Active() にも
// 映る。持たせないと、確認ダイアログの開閉のたびにタブへ「開け」「閉じろ」を戻す
// 往復が要り、その往復のどこかを書き忘れたタブだけが確認を飛ばす。
type Model struct {
	tab     int
	overlay page.Overlay
	st      page.StateMsg
	actions action.Set

	// pending は確認待ちの操作。y が押された時点で実行へ移す。
	pending pending
	// drain は進行中のドレイン停止。**ポインタで持つ**（drainRun の doc）。
	drain *drainRun
	// note は次の DoneMsg に添える補足。実行の前後で決まるため結果とは別に持つ。
	note note
	// report は状態行に出す直近の結果。空なら出さない。
	report string
	// seq は待機を始めるたびに増える通し番号。キャンセル後に届く古い結果を捨てる。
	seq int
}

// pending は確認待ちの操作と対象。
type pending struct {
	op      action.ID
	targets []runner.Runner
}

// New は制御部を組み立て、確認ダイアログと待機画面を Overlay へ登録する。
//
// 登録をここで行うのは、開く側と登録する側を離さないためである（種類の綴りを
// タブ側が知る必要が無くなる）。**戻り値の Cmd は呼び出し側まで返すこと**
// （page.Overlay.Register の doc。捨てると登録時に始まる処理が動かない）。
func New(tab int, o page.Overlay, st page.StateMsg) (Model, tea.Cmd) {
	confirm := o.Register(ConfirmKind, NewConfirmModal(st))
	drain := o.Register(DrainKind, NewDrainModal(st))
	return Model{
		tab:     tab,
		overlay: o,
		st:      st,
		actions: action.NewSet(st.Keys.Runner),
		pending: pending{op: action.Unknown, targets: nil},
		drain:   nil,
		note:    note{skipped: 0, canceled: false},
		report:  "",
		seq:     0,
	}, tea.Batch(confirm, drain)
}

// SetState は共有状態のスナップショットを反映する。
//
// 操作の表（action.Set）はタブが組んだものを受け取る。同じキー定義から 2 つ組むと
// 3 秒ごとに 11 要素の表を余分に作ることになり、可否の判定がタブとここで食い違う
// 余地も生まれる（action.Set の doc）。
//
// 待機画面の表示更新はここでは行わない。開いているモーダルへは page.Overlay が
// 共有状態をそのまま配るので、待機画面が自分で対象を引き直す（drainModal.refresh）。
func (m *Model) SetState(st page.StateMsg, actions action.Set) {
	m.st, m.actions = st, actions
}

// Status は状態行に出す直近の結果を返す。無ければ空文字を返す。
func (m Model) Status() string { return m.report }

// ClearStatus は直近の結果を消す。esc（1 つ前の状態へ戻る）で呼ぶ。
func (m *Model) ClearStatus() { m.report = "" }

// Result はモーダルが返した決定を処理する。
//
// **詳細画面の決定もここで受ける。** 詳細画面から選んだ操作の対象は、その詳細画面が
// 開いている runner 1 件だけである（一覧の一括選択は使わない）。詳細画面は「その
// runner に対する操作」の起点であり、開いている対象と実行される対象が食い違うと、
// 画面に出ている情報を見て承認したことにならないためである。一覧の直接キーが
// 一括選択を見るのと**意図的に違う**（screens.md の詳細画面 / FR-08）。
//
// 破壊的操作なら、詳細画面を閉じずに確認ダイアログを重ねる（page.Overlay は
// モーダルを重ねられる）。起点によって確認の強さを変えないためである（FR-45〜FR-47）。
func (m *Model) Result(msg page.ResultMsg) tea.Cmd {
	switch msg.Kind {
	case runnerdetail.Kind:
		c, ok := msg.Msg.(runnerdetail.ChosenMsg)
		if !ok {
			return nil
		}
		return m.Start(c.Action, []runner.Runner{c.Runner})
	case ConfirmKind:
		c, ok := msg.Msg.(dialog.ConfirmedMsg)
		if !ok {
			return nil
		}
		return m.confirmed(c.OK)
	case DrainKind:
		if _, ok := msg.Msg.(dialog.DrainCanceledMsg); !ok {
			return nil
		}
		return m.cancelDrain()
	default:
		return nil
	}
}

// Update は runnerop が自分宛に発行した Msg を処理する。
func (m *Model) Update(msg Msg) tea.Cmd {
	switch p := msg.Payload.(type) {
	case DoneMsg:
		m.report = describe(m.name(p.Op), p.Results, m.note)
		m.note = note{skipped: 0, canceled: false}
		return nil
	case drainStepMsg:
		return m.stepDrain(p)
	default:
		return nil
	}
}

// Start は操作を起動する。
//
// 破壊的操作（x / X / R）は確認ダイアログを開き、そうでなければ実行の Cmd を返す。
// ドレイン停止は待機画面を開く（startDrain）。
//
// **対象ごとに可否を再判定し、実行できないものは外す。** フッタが出す可否は
// カーソル位置の 1 台だけを見ており、一括選択した対象には操作できない runner
// （run.sh 直起動・ユニット不明）が混じりうる。判定は再実装せず action.Allow を
// 通す。理由の文言と判定の順を 1 箇所（svc.CanControl）に保つためである。
func (m *Model) Start(op action.ID, targets []runner.Runner) tea.Cmd {
	def, ok := m.def(op)
	if !ok {
		return nil
	}

	allowed, reason := m.filter(def, targets)
	m.note = note{skipped: len(targets) - len(allowed), canceled: false}
	if len(allowed) == 0 {
		m.report = def.Desc + ": " + noTargetText(len(targets), reason)
		return nil
	}

	switch {
	case op == action.Drain:
		return m.startDrain(allowed)
	case !needsConfirm(op):
		return m.run(op, allowed)
	default:
		m.pending = pending{op: op, targets: allowed}
		return m.overlay.Open(ConfirmKind, confirmInput(def, allowed))
	}
}

// confirmed は確認ダイアログの決定を処理する。
//
// **どちらの決定でもモーダルを 1 枚閉じる。** ダイアログ自身は閉じない設計であり
// （dialog.ConfirmedMsg の doc）、閉じる判断は開いた側の責任である。
func (m *Model) confirmed(ok bool) tea.Cmd {
	p := m.pending
	m.pending = pending{op: action.Unknown, targets: nil}
	m.overlay.Close()
	if !ok || len(p.targets) == 0 {
		return nil
	}
	return m.run(p.op, p.targets)
}

// needsConfirm は操作が確認ダイアログを経るかを返す。
//
// 経るのは x / X / R（と、この版に無い D / u）である。d（ドレイン停止）は待機の
// 開始にすぎず待機中にキャンセルできるため、s（開始）と E（切替）は元へ戻せる
// ため確認を求めない（screens.md の Runners タブの操作）。
func needsConfirm(op action.ID) bool {
	switch op {
	case action.Stop, action.Kill, action.Restart:
		return true
	default:
		return false
	}
}

// filter は対象のうち実行できるものだけを返す。最初に見つかった不可の理由も返す。
func (m Model) filter(def action.Def, targets []runner.Runner) ([]runner.Runner, string) {
	out := make([]runner.Runner, 0, len(targets))
	reason := ""
	for _, r := range targets {
		ok, why := action.Allow(def, r, m.st.Caps)
		if ok {
			out = append(out, r)
			continue
		}
		if reason == "" {
			reason = why
		}
	}
	return out, reason
}

// def は操作の定義（説明文・影響・破壊性）を返す。
func (m Model) def(op action.ID) (action.Def, bool) {
	for _, d := range m.actions.List() {
		if d.ID == op {
			return d, true
		}
	}
	return action.Def{ID: action.Unknown, Key: "", Desc: "", Impact: "", Destructive: false, Supported: false}, false
}

// name は操作の表示名を返す。結果の報告と確認の見出しに使う。
func (m Model) name(op action.ID) string {
	d, ok := m.def(op)
	if !ok {
		return op.String()
	}
	return d.Desc
}
