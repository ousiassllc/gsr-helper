// Package rowview は Runners タブの一覧の行を組み立てる。
//
// page/runners 本体から分けているのは 2 つの理由による。
//   - ここにあるのは検出結果（runner.Runner / runner.SvcState）と共有状態を
//     表示用の値へ落とす**純粋関数**であり、tea.Model を組み立てずに検証できる。
//   - 1 ディレクトリ 2000 行の上限に対して page/runners に余裕が無い。`_WORK` 列を
//     埋める（Issue #73）と上限を超えるため、上限値を緩めるのではなく分けた。
//
// 先例は page/disk/cleanview（同じくドメインの値を表示用の値へ落とす純粋関数）。
package rowview

import (
	"strings"

	"github.com/ousiassllc/gsr-helper/internal/runner"
	"github.com/ousiassllc/gsr-helper/internal/ui/keymap"
	"github.com/ousiassllc/gsr-helper/internal/ui/molecule/listrow"
	"github.com/ousiassllc/gsr-helper/internal/ui/organism/table"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
	"github.com/ousiassllc/gsr-helper/internal/ui/token"
)

// 区画の添字。runner の一覧の下に孤児ユニットの区画を置く（screens.md の Runners タブ）。
const (
	SectionRunners = 0
	SectionOrphans = 1
)

// orphanNote は孤児ユニットの行に添える注記（screens.md の Runners タブ）。
const orphanNote = "（対応ディレクトリなし）"

// Row は Runners タブの 1 行。
//
// organism.Table[T] は区画をまたいで同じ T を使う（区画ごとに型を変えられない）ため、
// runner の行と孤児ユニットの行を 1 つの型で表す。区画ごとに Render を分けてあるので、
// 行の種類と区画は必ず対応する。
type Row struct {
	Runner runner.Runner
	Orphan runner.SvcState
	// Work は _work 使用量の表示（Issue #73）。空なら "-" に縮退する。
	//
	// **行を組むときに写し取る。** 描画（renderRunner）が受け取るのは行だけで
	// 共有状態を見られないためである。
	Work     string
	IsOrphan bool
}

// NewTable は Runners タブの一覧を組み立てる。
func NewTable(keys keymap.Set, s token.Styles) table.Model[Row] {
	return table.New(keys.List, s, runnerSection(), orphanSection())
}

// runnerSection は runner 一覧の区画を返す。
func runnerSection() table.SectionInput[Row] {
	return table.SectionInput[Row]{
		Title:      "",
		Columns:    token.RunnerColumns(),
		Rules:      token.RunnerColumnRules(),
		Render:     renderRunner,
		ID:         func(r Row) string { return r.Runner.Dir },
		Match:      matchRunner,
		Disabled:   nil,
		Selectable: true,
	}
}

// orphanSection は孤児ユニットの区画を返す。
//
// 選択できないのは、この版で孤児ユニットに対して行える操作が無いためである
// （ユニットの削除はサービス制御の Issue の担当）。絞り込みの対象にもしない。
// 孤児ユニットは異常の報告であり、runner 名で絞り込んだ結果から消えると
// 見落とすためである。
func orphanSection() table.SectionInput[Row] {
	return table.SectionInput[Row]{
		Title:   "孤児ユニット",
		Columns: token.OrphanColumns(),
		// runner の一覧と同じ落とし方を使う（同じタブの中で列の落ち方が
		// 変わると桁の対応が読み取れない）。UNIT / NOTE は Drop に無いので
		// 幅が足りない場合は末尾（NOTE）から落ちる。
		Rules:      token.RunnerColumnRules(),
		Render:     renderOrphan,
		ID:         func(r Row) string { return r.Orphan.Unit },
		Match:      nil,
		Disabled:   nil,
		Selectable: false,
	}
}

// renderRunner は runner の行をセル列に変換する。
//
// この区画は選択不可の行を持たない（Disabled が nil）ため RowInput.Reason は常に空で、
// 理由のセルも持たない。
func renderRunner(in table.RowInput[Row]) []string {
	return listrow.RunnerRow(View(in.Item), in.Cols, in.Styles)
}

// renderOrphan は孤児ユニットの行をセル列に変換する。
func renderOrphan(in table.RowInput[Row]) []string {
	return listrow.OrphanRow(listrow.OrphanView{
		Unit:   in.Item.Orphan.Unit,
		Active: in.Item.Orphan.Active,
		Sub:    in.Item.Orphan.Sub,
		Note:   orphanNote,
	}, in.Cols, in.Styles)
}

// matchRunner は絞り込みの一致判定。名前とスコープを対象にする。
//
// 大文字小文字を無視するのは、runner 名が build01-1 のように小文字で、スコープが
// org:Foo のように大文字を含みうるためである。
func matchRunner(r Row, q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(strings.ToLower(r.Runner.Name()), q) ||
		strings.Contains(strings.ToLower(r.Runner.Scope.String()), q)
}

// View は行を表示用の構造体に落とす。
//
// LatestVersion と Work は空にする。最新版の取得は GitHub API を使う機能、_work の
// 集計はディスクの機能の担当であり、未取得を「最新」「0 バイト」と示さないためである
// （molecule 側は空の値を "-" として描く）。
func View(row Row) listrow.RunnerView {
	r := row.Runner
	v := listrow.RunnerView{
		Name:          r.Name(),
		Scope:         r.Scope.String(),
		Managed:       r.Managed.String(),
		SvcActive:     "",
		SvcSub:        "",
		Busy:          r.Busy(),
		Elapsed:       r.JobElapsed(),
		Version:       r.Version,
		LatestVersion: "",
		Work:          row.Work,
		Warn:          warned(r),
		SvcUnknown:    svcUnknown(r),
	}
	if r.Svc != nil {
		v.SvcActive, v.SvcSub = r.Svc.Active, r.Svc.Sub
	}
	return v
}

// svcUnknown はユニットはあるが状態を取得できなかったかを返す。
//
// systemctl show が失敗したユニットは値の無い SvcState として渡ってくる
// （internal/runner のプレースホルダ）。ユニットが無い場合（Svc == nil）と同じ
// 記号で描くと、仕様が書き分けている 2 つの状態を読み分けられない。
func svcUnknown(r runner.Runner) bool {
	return r.Svc != nil && r.Svc.Active == ""
}

// warned は行に注意記号を出すかを返す。
//
// 記号の意味は「この行は要注意である」の 1 つだけで、次のいずれかに当てはまるときに
// 付ける。すなわち、サービスが異常終了している、サービス制御ができないか可否を判定
// できない（systemd 管理外、または systemd の管理状態が判定できなかった）、バージョンが
// 読めない、のいずれかである。
//
// 管理状態が判定できない runner にも付けるのは、run.sh 直起動と分かっている runner より
// 要注意だからである。ユニット一覧が取れておらず、サービス制御の可否すら決められない
// （page.Allow はこの状態で開始・停止・再起動・enable の切替を塞ぐ）。
// バージョンの新旧には最新版の取得が必要で、権限異常の判定は doctor の担当なので含めない。
func warned(r runner.Runner) bool {
	if r.Svc != nil && (r.Svc.Active == "failed" || r.Svc.Sub == "failed") {
		return true
	}
	return r.Managed == runner.ManagedStandalone ||
		r.Managed == runner.ManagedUnavailable ||
		r.Version == ""
}

// Runners は検出結果を runner の行に変換する。
//
// _work 使用量は共有状態（page.DiskState）から引く。親が別周期で集計しており
// （Issue #73）、まだ終わっていない runner は空文字のまま "-" に縮退する。
func Runners(runners []runner.Runner, ds page.DiskState) []Row {
	out := make([]Row, 0, len(runners))
	for _, r := range runners {
		out = append(out, Row{
			Runner: r, Orphan: runner.SvcState{},
			Work: ds.WorkText(r.Dir), IsOrphan: false,
		})
	}
	return out
}

// Orphans は孤児ユニットを行に変換する。
func Orphans(units []runner.SvcState) []Row {
	out := make([]Row, 0, len(units))
	for _, u := range units {
		out = append(out, Row{Runner: runner.Runner{}, Orphan: u, Work: "", IsOrphan: true})
	}
	return out
}
