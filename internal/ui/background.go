package ui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/doctor"
)

// 共有状態のうち、起動シーケンスの外で非同期に取りに行くものの駆動を集める
// （前提チェック = FR-44、_work 使用量 = Issue #73、保有スコープ = Issue #79）。
// 取得の実装と進行状況の管理はサブパッケージ（hostreq / workscan / ghscope）に
// あり、ここにあるのは「いつ始めるか」だけである。
//
// **どちらも 3 秒ごとの再検出サイクルには載せない。** 走査は分単位、API は往復を
// 要するため、既定タブ（Runners）の更新周期に載せると「起動から一覧表示まで
// 1 秒以内」（non-functional.md）を壊す。契機は最初の検出成功と、手動の再読み込み
// （r。_work のみ）である。

// startBackground は最初の検出が成功した直後に 1 度だけ走らせる取得を並べる。
//
// 検出の成功を待つのは、_work の集計に runner 一覧が要るためである。起動時の前提
// チェック（hostreq）と同じ契機だが、あちらは runner ごとの sudo 判定という別の
// 理由でこの契機を選んでいるので、条件は共有しない。
//
// **束ねずにスライスで返す。** 呼び出し側（applyDiscovered）は共有状態の配布と
// 合わせて 1 段の tea.Batch にする。ここで束ねると Batch が入れ子になり、配布ぶんの
// ChromeMsg を親の検証が取り出せなくなる。
func (a *App) startBackground() []tea.Cmd {
	// **3 つとも「1 度きり」を自分で覚えている。** 呼び出し元の契機は「検出が成功した
	// 周期」であり初回とは限らないので（discovery.Reconcile の StartHostReq は成功の
	// たびに真になる）、ここで数え直さない。
	cmds := []tea.Cmd{
		a.hr.StartOnce(doctor.Input{Runners: a.disc.Result().Runners, Caps: a.caps, Exec: a.ex}),
		a.work.StartOnce(a.disc.Result().Runners),
		a.scopes.Start(a.ex, a.caps.GitHubToken),
	}

	out := make([]tea.Cmd, 0, len(cmds))
	for _, cmd := range cmds {
		if cmd != nil {
			out = append(out, cmd)
		}
	}
	return out
}
