package doctor

import (
	"cmp"
	"context"
	"slices"
	"sync"

	"github.com/ousiassllc/gsr-helper/internal/doctor/check"
)

// Run は checks を並列に実行し、結果を表示順に整列して返す（FR-32）。
//
// 期限は呼び出し側が ctx で切る。1 コマンドあたりの待ちは check.ProbeTimeout が
// 押さえるが、項目数ぶん直列に積み上がる経路（1 つの項目が runner ごとに何本も
// コマンドを出す）まではここで抑えられないためである。
//
// **panic は握り潰さない。** 項目の実装の誤りを FAIL の 1 行に化けさせると、
// 「対処しても直らないチェック」として残り続け、原因がどこにも出ない。
func Run(ctx context.Context, in Input, checks []Check) []CheckResult {
	out := make([][]CheckResult, len(checks))

	var wg sync.WaitGroup
	for i, c := range checks {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out[i] = c.Run(ctx, in)
		}()
	}
	wg.Wait()

	var all []CheckResult
	for _, g := range out {
		all = append(all, g...)
	}
	return Sort(all)
}

// Sort は結果を表示順に整列して返す。
//
// 並べる順は 分類 → 識別子 → 対象 である。並列実行の完了順に並べると、
// 再実行のたびに行が入れ替わってカーソルの位置が意味を失う。
//
// 公開するのは、個別の再実行（Replace）が同じ規則で並べ直す必要があるためで
// ある。UI 側に並べ替えを書くと、整列の規則が 2 箇所へ分かれる。
//
// **引数は書き換えない。** 呼び出し側（page）は結果のスナップショットを保持した
// まま描画に使うので、その場で並べ替えると描画中のスライスが動く。並べ替えの
// 費用より、共有された値を壊さないことを採る。
func Sort(results []CheckResult) []CheckResult {
	all := slices.Clone(results)
	slices.SortStableFunc(all, func(a, b CheckResult) int {
		if n := cmp.Compare(check.CategoryRank(a.Category), check.CategoryRank(b.Category)); n != 0 {
			return n
		}
		if n := cmp.Compare(a.Category, b.Category); n != 0 {
			return n
		}
		if n := cmp.Compare(a.ID, b.ID); n != 0 {
			return n
		}
		return cmp.Compare(a.Target, b.Target)
	})
	return all
}

// Startup は起動時の自動実行（FR-44）の対象だけを返す。
//
// 絞り込みで済ませるのは、起動時と Doctor タブで同じ実装を通すためである
// （components/overview.md）。別経路にすると、片方だけが直った状態を作れる。
func Startup(checks []Check) []Check {
	out := make([]Check, 0, len(checks))
	for _, c := range checks {
		if c.Startup() {
			out = append(out, c)
		}
	}
	return out
}

// ByID は識別子の一致する項目だけを返す（個別の再実行。FR-34）。
func ByID(checks []Check, id string) []Check {
	out := make([]Check, 0, 1)
	for _, c := range checks {
		if c.ID() == id {
			out = append(out, c)
		}
	}
	return out
}

// Summary は判定ごとの件数（screens.md の `OK 12  WARN 3  FAIL 3  SKIP 1`）。
type Summary struct {
	OK   int
	Warn int
	Fail int
	Skip int
}

// Bad は対処が要る件数（WARN + FAIL）を返す。
func (s Summary) Bad() int { return s.Warn + s.Fail }

// Total は全件数を返す。
func (s Summary) Total() int { return s.OK + s.Warn + s.Fail + s.Skip }

// Count は結果を判定ごとに数える。
func Count(results []CheckResult) Summary {
	var s Summary
	for _, r := range results {
		switch r.Status {
		case OK:
			s.OK++
		case Warn:
			s.Warn++
		case Fail:
			s.Fail++
		case Skip:
			s.Skip++
		}
	}
	return s
}

// Replace は既存の結果から id の行を取り除き、再実行の結果で置き換えて返す。
//
// 個別の再実行（FR-34）に使う。1 項目が返す行数は実行のたびに変わりうる
// （runner が増減する）ので、同じ識別子の行をまとめて捨ててから入れ替える。
// 差分を取って部分的に更新すると、消えた対象の行が残る。
//
// 元のスライスは書き換えない。呼び出し側（page）が保持しているスナップショットを
// 別の goroutine から壊さないためである。
func Replace(results, fresh []CheckResult, id string) []CheckResult {
	out := make([]CheckResult, 0, len(results)+len(fresh))
	for _, r := range results {
		if r.ID != id {
			out = append(out, r)
		}
	}
	out = append(out, fresh...)
	return Sort(out)
}
