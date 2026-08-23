package ui

import (
	"slices"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/runner"
	"github.com/ousiassllc/gsr-helper/internal/ui/discovery"
)

// tickMsg は自動更新の契機。非公開にして、外から自動更新を駆動できないようにする。
type tickMsg struct{}

// tick は次の自動更新を予約する Cmd を返す。
func (a App) tick() tea.Cmd {
	return tea.Tick(a.refresh(), func(time.Time) tea.Msg { return tickMsg{} })
}

// firstTick は待たずに tickMsg を返す Cmd。Init が最初の周期を始めるのに使う。
//
// Init から直に検出を始めない理由は Init の doc を参照。
func firstTick() tea.Cmd {
	return func() tea.Msg { return tickMsg{} }
}

// onTick は自動更新 1 周期分の Cmd を返す。
//
// **実行中の検出があるときは新しい検出を始めない。** 自動更新間隔（既定 3 秒、下限
// 1 秒）は 1 回の検出に許す時間（discovery.Budget = 15 秒）より短いため、無条件に
// 発行すると最大 5 周期が重なる。1 回の検出は systemctl show を showConcurrency（8）
// 並列で撒くので、systemctl が応答しない——まさに検出が長引くその状況で——最大
// 40 プロセスが同時に走ることになる。次の周期は必ず予約するので、遅い検出が
// 終わればそのまま自動更新は続く。
func (a *App) onTick() tea.Cmd {
	if a.inflight > 0 {
		return a.tick()
	}
	return tea.Batch(a.discover(), a.tick())
}

// applyDiscovered は検出結果を取り込み、共有状態を配る Cmd を返す。周期の追い抜き・
// 部分結果の扱い・起動時の前提チェック（FR-44）を許可する条件は discovery.Reconcile
// の doc を参照（判断はそちらへ寄せ、ここは Outcome をフィールドへ書き写して配る
// だけである）。
//
// 発行しないときは束ねない。**共有状態の配布だけの Cmd の形を変えない**ためで
// ある（親の検証は 1 段展開で ChromeMsg を拾う）。
func (a *App) applyDiscovered(msg discovery.Msg) tea.Cmd {
	if a.inflight > 0 {
		a.inflight--
	}

	out := discovery.Reconcile(a.applied, a.result, a.err, msg)
	if out.Stale {
		return nil
	}
	a.applied, a.result, a.err = out.Applied, out.Result, out.Err

	cmd := a.distribute()
	if !out.StartHostReq {
		return cmd
	}
	// 一覧を採れた最初の周期で、起動シーケンスの外へ回した取得を始める
	// （前提チェック = FR-44、_work 使用量 = Issue #73、保有スコープ = Issue #79）。
	// **失敗した周期では発行しない。** 空の一覧で 1 度きりの実行を使い切ると、
	// runner ごとに判定するものがセッション中一度も走らない。
	//
	// 発行するものが無いときは束ねない。**共有状態の配布だけの Cmd の形を変えない**
	// ためである（親の検証は 1 段展開で ChromeMsg を拾う）。
	extra := a.startBackground()
	if len(extra) == 0 {
		return cmd
	}
	return tea.Batch(append([]tea.Cmd{cmd}, extra...)...)
}

// discover は runner を検出する Cmd を返す。中身は discovery.Start に委ねる（discovery.go の package doc を参照）。
//
// 引数に必要な値を Cmd の外で写し取るのは、Cmd が別 goroutine で走る間に親 Model の
// 状態が書き換わっても、検出の入力が変わらないようにするためである。
// 実行中の本数（inflight）と通し番号（seq）を進めるためポインタで受け取る。
// 発行した検出を親が数えられないと二重起動を防げない（onTick を参照）。
func (a *App) discover() tea.Cmd {
	a.inflight++
	a.seq++
	seq := a.seq
	ex := discovery.Exec(a.caps.Systemd, a.ex)
	roots := slices.Clone(a.opts.Roots)
	depth := a.cfg.ScanDepth

	return discovery.Start(seq, runner.Options{Roots: roots, Depth: depth, Exec: ex})
}

// refresh は自動更新間隔を決める。中身は discovery.Interval に委ねる（discovery.go の package doc を参照）。
func (a App) refresh() time.Duration {
	return discovery.Interval(a.opts.Refresh, a.cfg.RefreshDuration())
}
