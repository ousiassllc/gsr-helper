package ui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/appconfig"
	"github.com/ousiassllc/gsr-helper/internal/ui/discovery"
	"github.com/ousiassllc/gsr-helper/internal/ui/startup"
)

// 検出の駆動のうち、親 Model にしか決められないものだけをここに置く。周期の管理
// （実行中は重ねない・追い抜かれた周期を捨てる・次の Tick を予約する）は
// discovery.State が持つ（Issue #139）。

// onTick は自動更新 1 周期分の Cmd を返す。始めるかどうかと次の Tick の予約は
// discovery.State.OnTick の doc を参照。
func (a *App) onTick() tea.Cmd { return a.disc.OnTick(a.input()) }

// applyDiscovered は検出結果を取り込み、共有状態を配る Cmd を返す。周期の追い抜き・
// 部分結果の扱い・起動時の前提チェック（FR-44）を許可する条件は discovery.Reconcile
// の doc を参照（判断はそちらへ寄せ、ここは Outcome を見て配るだけである）。
//
// 発行しないときは束ねない。**共有状態の配布だけの Cmd の形を変えない**ためで
// ある（親の検証は 1 段展開で ChromeMsg を拾う）。
func (a *App) applyDiscovered(msg discovery.Msg) tea.Cmd {
	out := a.disc.Apply(msg)
	if out.Stale {
		return nil
	}

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
	extra := a.bg.StartAll(startup.Input{Runners: a.disc.Result().Runners, Caps: a.caps, Exec: a.ex})
	if len(extra) == 0 {
		return cmd
	}
	return tea.Batch(append([]tea.Cmd{cmd}, extra...)...)
}

// input は 1 周期分の検出の入力を組み立てる。
//
// **周期ごとに組み立て直す。** 走査ルートも自動更新間隔も設定ファイルの値に依存し、
// その値は Config タブの保存で書き換わる（app.go の page.ConfigSavedMsg）。起動時に
// 畳んだ結果を持ち回ると、設定ファイルには書けているのに検出の入力が再起動まで
// 古いままになる（Issue #132）。
//
// 自動更新間隔の決め方は discovery.Interval、systemctl 不在時の縮退は discovery.Exec
// が持つ（どちらも親の状態を読まなくても答えが決まる）。
func (a App) input() discovery.Input {
	return discovery.Input{
		Every: discovery.Interval(a.opts.Refresh, a.cfg.RefreshDuration()),
		Roots: a.scanRoots(),
		Depth: a.cfg.ScanDepth,
		Exec:  discovery.Exec(a.caps.Systemd, a.ex),
	}
}

// scanRoots は走査に渡す追加ルートを返す（Issue #132）。
//
// **合成は周期ごとにやり直す。** 設定ファイルの scan_roots は Config タブの保存で
// 書き換わり、新しい値は page.ConfigSavedMsg で a.cfg に載る（app.go の case）。
// 起動時に合成した結果を持ち回ると、設定ファイルには書けているのに走査の入力が
// 再起動まで古いままになる。
//
// **--root は保存後も効き続ける。** 明示的に渡したルートを設定の保存で消すと、
// 起動コマンドを変えていないのに走査対象が減る。順序は scan_roots → --root
// （docs/components/overview.md の走査ルートの合成）。
//
// 返り値は MergeScanRoots が毎回作る新しいスライスなので、Cmd が別 goroutine で
// 読んでも親 Model の状態とは切り離されている（discovery.State.Start の doc）。
func (a App) scanRoots() []string {
	return appconfig.MergeScanRoots(a.cfg.ScanRoots, a.opts.Roots)
}
