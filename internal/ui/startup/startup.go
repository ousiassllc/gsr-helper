// Package startup は「起動後に 1 度だけ取りに行く共有状態」の駆動をまとめる
// （前提チェック = FR-44、_work 使用量 = Issue #73、保有スコープ = Issue #79）。
//
// 取得の実装と進行状況の管理はサブパッケージ（hostreq / workscan / ghscope）に
// あり、ここにあるのは「いつ始めるか」だけである。親 Model からこの判断を出せる
// のは、契機（最初の検出成功）と入力（runner 一覧・Caps・Executor）が親の非公開な
// 状態に触れなくても決まるためである（Issue #148）。
//
// **3 つとも 3 秒ごとの再検出サイクルには載せない。** 走査は分単位、API は往復を
// 要するため、既定タブ（Runners）の更新周期に載せると「起動から一覧表示まで
// 1 秒以内」（non-functional.md）を壊す。契機は最初の検出成功と、手動の再読み込み
// （r。_work のみ。親の keys.go が State を直に叩く）である。
package startup

import (
	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/appconfig"
	"github.com/ousiassllc/gsr-helper/internal/doctor"
	"github.com/ousiassllc/gsr-helper/internal/exec"
	"github.com/ousiassllc/gsr-helper/internal/runner"
	"github.com/ousiassllc/gsr-helper/internal/ui/ghscope"
	"github.com/ousiassllc/gsr-helper/internal/ui/hostreq"
	"github.com/ousiassllc/gsr-helper/internal/ui/workscan"
)

// State は起動後に 1 度だけ取りに行く 3 つの共有状態をまとめて持つ。
//
// **3 つを 1 つに畳むのは、性質と契機が同じだからである。** どれも「最初の検出
// 成功で 1 度だけ始め、結果が届いたら共有状態へ載せる」形をしており、親 Model は
// 3 つを個別に知る必要が無い（Issue #148）。
//
// **フィールドを公開してあるのは、親が結果と差し替え口へ直に触るためである。**
// 「1 度きり」の管理はそれぞれの State が自分で持つので（StartOnce / Start）、
// ここで包み直すと同じ不変条件の写しが増えるだけになる。
type State struct {
	// HostReq は起動時のジョブ実行の前提チェック（FR-44）の件数と進行状況。
	HostReq hostreq.State
	// Work は runner ごとの _work 使用量とその集計の進行状況（Issue #73）。
	Work workscan.State
	// Scopes はトークンの保有スコープと取得の進行状況（Issue #79）。
	Scopes ghscope.State
}

// Input は 1 度の駆動に要る入力。親 Model が周期ごとに組み立てて渡す。
type Input struct {
	// Runners は直近の検出で採れた runner 一覧。空だと前提チェックも集計も
	// 発行されない（それぞれの Start が nil を返す）。
	Runners []runner.Runner
	// Caps は能力判定。前提チェックの入力と、トークンの有無の判定に使う。
	Caps appconfig.Caps
	// Exec は外部コマンドの実行口。
	Exec exec.Executor
}

// StartAll は最初の検出が成功した直後に 1 度だけ走らせる取得を並べる。
//
// 検出の成功を待つのは、_work の集計に runner 一覧が要るためである。起動時の前提
// チェック（hostreq）と同じ契機だが、あちらは runner ごとの sudo 判定という別の
// 理由でこの契機を選んでいるので、条件は共有しない。
//
// **3 つとも「1 度きり」を自分で覚えている。** 呼び出し元の契機は「検出が成功した
// 周期」であり初回とは限らないので（discovery.Reconcile の StartHostReq は成功の
// たびに真になる）、ここで数え直さない。
//
// **束ねずにスライスで返す。** 呼び出し側（applyDiscovered）は共有状態の配布と
// 合わせて 1 段の tea.Batch にする。ここで束ねると Batch が入れ子になり、配布ぶんの
// ChromeMsg を親の検証が取り出せなくなる。
func (s *State) StartAll(in Input) []tea.Cmd {
	cmds := []tea.Cmd{
		s.HostReq.StartOnce(doctor.Input{Runners: in.Runners, Caps: in.Caps, Exec: in.Exec}),
		s.Work.StartOnce(in.Runners),
		s.Scopes.Start(in.Exec, in.Caps.GitHubToken),
	}

	out := make([]tea.Cmd, 0, len(cmds))
	for _, cmd := range cmds {
		if cmd != nil {
			out = append(out, cmd)
		}
	}
	return out
}
