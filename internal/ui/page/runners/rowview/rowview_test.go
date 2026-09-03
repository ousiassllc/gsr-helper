package rowview_test

import (
	"errors"
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/runner"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/pagetest"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/runners/rowview"
)

// errScan は集計に失敗した runner を表すためのエラー。
var errScan = errors.New("集計に失敗")

// Runners タブの `_WORK` 列（Issue #73）を検証する。
//
// 集計は親が別周期で駆動するため、開いた直後は未集計である。**未集計・集計失敗が
// `-` に縮退し、一覧そのものは失敗しない**ことがこの列の要点である。

// workRunner は Dir だけを持つ runner を返す。`_WORK` は Dir で引く。
func workRunner(dir string) runner.Runner {
	return runner.Runner{Dir: dir}
}

func TestRunnerRowsFillWorkFromSharedState(t *testing.T) {
	ds := page.DiskState{Work: map[string]page.WorkUsage{
		"/opt/runners/a": {Bytes: 2048},
		"/opt/runners/c": {Bytes: 0},
	}}
	ds.Work["/opt/runners/b"] = page.WorkUsage{Err: errScan}

	rows := rowview.Runners([]runner.Runner{
		workRunner("/opt/runners/a"),
		workRunner("/opt/runners/b"),
		workRunner("/opt/runners/c"),
		workRunner("/opt/runners/d"),
	}, ds)

	tests := []struct {
		name string
		i    int
		want string
	}{
		{name: "集計できた runner は使用量が入る", i: 0, want: "2.0K"},
		{name: "集計に失敗した runner は空（`-` へ縮退）", i: 1, want: ""},
		{name: "0 バイトはそのまま出す（空の _work は有効な値）", i: 2, want: "0B"},
		{name: "未集計の runner は空（`-` へ縮退）", i: 3, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := rows[tt.i].Work; got != tt.want {
				t.Errorf("work = %q, want %q", got, tt.want)
			}
		})
	}
}

// 未集計でも一覧は描ける（列が `-` になるだけ）。
func TestRunnerRowRendersDashWhenWorkIsUnknown(t *testing.T) {
	rows := rowview.Runners([]runner.Runner{workRunner("/opt/runners/a")}, page.DiskState{})

	if got := rowview.View(rows[0]).Work; got != "" {
		t.Errorf("RunnerView.Work = %q, want 空（molecule 側が - に落とす）", got)
	}
}

// 孤児ユニットの行は _work を持たない。
func TestOrphanRowsHaveNoWork(t *testing.T) {
	rows := rowview.Orphans([]runner.SvcState{{Unit: "actions.runner.x.service"}})

	if got := rows[0].Work; got != "" {
		t.Errorf("孤児ユニットに work が入っている: %q", got)
	}
}

// 行の注意記号（`Warn`）と SVC 列の不明表示（`SvcUnknown`）を検証する。
//
// どちらも `warned` / `svcUnknown` が決めており、page/runners のタブ全体を描いて
// 記号の文字列を探さなくても固定できる。判定そのものはここが持つためである。

// systemd の管理状態が判定できない runner の行にも注意記号を出す。
//
// この状態の runner は「ユニットが無い」と区別できないまま run.sh 直起動として
// 表示されていた行であり、注意記号もそれで付いていた。管理状態が分からない方が
// 直起動と分かっているより要注意なので、記号を落としてはならない。
func TestUnavailableManagedRowIsWarned(t *testing.T) {
	rows := rowview.Runners([]runner.Runner{pagetest.UnavailableRunner()}, page.DiskState{})

	if !rowview.View(rows[0]).Warn {
		t.Error("管理状態が判定できない行の RunnerView.Warn が偽である（注意記号が出ない）")
	}
}

// 状態を取得できなかったユニットは、ユニットが無い runner と SVC 列で書き分ける。
//
// systemctl show が失敗したユニットは値の無い状態として渡ってくる（internal/runner の
// プレースホルダ）。同じ "-" で描くと「サービス登録されていない」と誤読される。
func TestUnknownServiceStateIsDistinguished(t *testing.T) {
	unknown := pagetest.SampleRunner()
	// systemctl show が失敗したユニットのプレースホルダ（値が無い SvcState）。
	unknown.Svc = &runner.SvcState{Unit: unknown.UnitName}

	rows := rowview.Runners([]runner.Runner{unknown, pagetest.StandaloneRunner()}, page.DiskState{})

	if !rowview.View(rows[0]).SvcUnknown {
		t.Error("状態が取れなかった行の RunnerView.SvcUnknown が偽である")
	}
	// ユニットを持たない runner は「取れなかった」ではない（同じ記号で描かせない）。
	if rowview.View(rows[1]).SvcUnknown {
		t.Error("ユニットが無い runner まで状態不明として扱っている")
	}
}
