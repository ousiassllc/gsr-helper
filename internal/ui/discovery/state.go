package discovery

import (
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/exec"
	"github.com/ousiassllc/gsr-helper/internal/runner"
)

// TickMsg は自動更新の契機。
//
// **かつては親 Model の非公開な型（ui.tickMsg）だった。** 「外から自動更新を駆動
// できないようにする」ためだったが、周期の管理をこのパッケージへ移すにあたって
// 公開する側へ反転させた。理由は 2 つある。
//   - 非対称だった。同じく親が受け取る hostreq.Msg / workscan.Msg / ghscope.Msg は
//     いずれも公開されており、契機の型だけを隠しても揃わない。
//   - 実効性が限定的である。State.Start は実行中なら nil を返すので、外から余計な
//     TickMsg を撒いても起きるのは高々 1 周期ぶんの前倒しであり、多重起動を防ぐ
//     判定（Start の doc の 40 プロセス）は破れない。
type TickMsg struct{}

// Input は 1 周期分の入力。
//
// **appconfig の型は持ち込まない。** 設定ファイルとフラグの合成・既定値の解決は
// 親 Model の仕事であり、ここへ appconfig を import すると依存グラフに
// ui/discovery → appconfig の辺が増える（docs/components/overview.md の依存グラフは
// 辺の欠落が無いことを明記している）。
type Input struct {
	// Every は次の Tick までの間隔。親が Interval で解決済み。
	Every time.Duration
	// Roots は追加の走査ルート。**親が周期ごとに合成し直す**（ui.App.scanRoots。
	// Issue #132）。
	Roots []string
	// Depth はルート配下を掘る深さ。
	Depth int
	// Exec は systemd 参照に使う Executor。親が Exec で縮退済み（systemctl が
	// 無い環境では nil）。
	Exec exec.Executor
}

// State は検出周期の進行状況。親 Model はこれを 1 つ持ち、実行中の本数・通し番号・
// 取り込み済みの周期と、その周期の結果の管理をこちらへ預ける。
//
// **親に int と error を並べさせない。** 「実行中は重ねない」「古い周期を捨てる」は
// 検出の側の不変条件であり、持ち主が離れると片方だけを更新する経路ができる
// （workscan.State と同じ判断）。
type State struct {
	// inflight は実行中の検出の本数。0 でない間は新しい検出を始めない（Start）。
	inflight int
	// seq は発行した検出の通し番号、applied は取り込んだ結果の番号。
	// 古い周期の結果で新しい一覧を上書きしないために持つ（Msg.Seq）。
	seq     int
	applied int

	result runner.Result
	err    error
}

// Result は取り込み済みの一覧を返す。
func (s *State) Result() runner.Result { return s.result }

// Err は取り込み済みの検出のエラーを返す（状態行に出す）。
func (s *State) Err() error { return s.err }

// Busy は実行中の検出があるかを返す。**手動の再読み込み（r）が案内を出すかの
// 判定に使う。** Start は始めなかったことを nil でしか返さないため、始めなかった
// 理由（実行中か否か）は呼び出し側からこれで見分ける。
func (s *State) Busy() bool { return s.inflight > 0 }

// Seq は発行した検出の通し番号を返す。**「検出を重ねていない」の検証に使う。**
// 実行中の本数そのものは公開しない（外から数え直させないため）。
func (s *State) Seq() int { return s.seq }

// FirstTick は待たずに TickMsg を返す Cmd。Init が最初の周期を始めるのに使う。
//
// Init から直に検出を始めない理由は ui.App.Init の doc を参照。
func FirstTick() tea.Cmd {
	return func() tea.Msg { return TickMsg{} }
}

// Tick は次の自動更新を every 後に予約する Cmd を返す。
func Tick(every time.Duration) tea.Cmd {
	return tea.Tick(every, func(time.Time) tea.Msg { return TickMsg{} })
}

// OnTick は自動更新 1 周期分の Cmd を返す。
//
// **次の Tick は必ず予約する。** 検出を始めなかった周期で予約を落とすと、自動更新が
// そこで止まったままになる。始めるかどうかの判断は Start に委ねる。
func (s *State) OnTick(in Input) tea.Cmd {
	return tea.Batch(s.Start(in), Tick(in.Every))
}

// Start は検出 1 回分の Cmd を返す。始めなかった場合は nil を返す。
//
// **実行中の検出があるときは新しい検出を始めない。** 自動更新間隔（既定 3 秒、下限
// 1 秒）は 1 回の検出に許す時間（Budget = 15 秒）より短いため、無条件に発行すると
// 最大 5 周期が重なる。1 回の検出は systemctl show を showConcurrency（8）並列で
// 撒くので、systemctl が応答しない——まさに検出が長引くその状況で——最大 40
// プロセスが同時に走ることになる。次の周期は必ず予約される（OnTick）ので、遅い
// 検出が終わればそのまま自動更新は続く。手動の再読み込み（r）の連打も同じ判定で
// 塞ぐ。
//
// 入力（in）は Cmd の外で写し取ってから渡す。Cmd が別 goroutine で走る間に親 Model
// の状態が書き換わっても、検出の入力が変わらないようにするためである。
func (s *State) Start(in Input) tea.Cmd {
	if s.Busy() {
		return nil
	}
	s.inflight++
	s.seq++
	return Start(s.seq, runner.Options{Roots: in.Roots, Depth: in.Depth, Exec: in.Exec})
}

// Apply は検出 1 周期分の結果を取り込み、その扱い（Outcome）を返す。
//
// 周期の追い抜き・部分結果の扱い・起動時の前提チェック（FR-44）を許可する条件の
// 判断は Reconcile が持つ。ここはその結果を State へ書き写すだけである。捨てる
// 周期（Outcome.Stale）でも実行中の本数は戻す——数えているのは発行した Cmd の
// 本数であり、結果を採るかどうかとは別だからである。
func (s *State) Apply(msg Msg) Outcome {
	if s.inflight > 0 {
		s.inflight--
	}

	out := Reconcile(s.applied, s.result, s.err, msg)
	if out.Stale {
		return out
	}
	s.applied, s.result, s.err = out.Applied, out.Result, out.Err
	return out
}
