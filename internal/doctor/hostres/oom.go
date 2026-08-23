package hostres

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/ousiassllc/gsr-helper/internal/doctor/check"
	"github.com/ousiassllc/gsr-helper/internal/runner"
)

// oomWindow は履歴を遡る幅。直近 1 日に絞るのは、古い記録を出し続けても
// 「対処したのに消えない行」になるだけだからである。
const oomWindow = 24 * time.Hour

// journalSinceLayout は journalctl --since が受け付ける時刻の書式。
const journalSinceLayout = "2006-01-02 15:04:05"

// oomMarkers はカーネルログ上の OOM Killer の痕跡。
//
// 3 つ挙げるのはカーネルのバージョンで文言が違うためである。1 つに絞ると、
// 特定のバージョンでだけ検出できない。
var oomMarkers = []string{"Out of memory", "oom-kill", "Killed process"}

// detailLineLimit は Detail に載せる 1 行の上限。カーネルログの 1 行は長い。
const detailLineLimit = 200

// impactOOM は OOM の履歴が示すもの。
const impactOOM = "ジョブが原因不明で失敗している場合、メモリ不足による強制終了が原因の可能性があります。"

// oomCheck はカーネルログ上の OOM Killer の履歴を判定する。
//
// **このコマンドは監査ログに記録する。** 同じ journalctl でも、ログ追従
// （internal/logs の 2 秒ごとの journalctl -u）は他のレコードを押し流すため
// 記録対象外にしてある。こちらは診断 1 回につき 1 本なので押し流しは起きない。
// 記録対象かどうかはコマンド名ではなく発行元で決まる（security.md）。
type oomCheck struct{}

func (oomCheck) ID() string       { return "history.oom" }
func (oomCheck) Category() string { return check.CatHistory }
func (oomCheck) Startup() bool    { return false }

// Run は OOM の痕跡を runner ごとにまとめて返す。
func (c oomCheck) Run(ctx context.Context, in check.Input) []check.Result {
	if !in.Caps.Journal || !in.Has("journalctl") {
		return one(check.Skipped(c, "OOM Killer の履歴",
			"journalctl がありません（カーネルログを読めません）。"))
	}

	since := in.Clock().Add(-oomWindow).Format(journalSinceLayout)
	res, err := in.Probe(ctx, "doctor.oom", "journalctl", "-k", "--since", since, "--no-pager")
	if err != nil || res.ExitCode != 0 {
		return one(check.Skipped(c, "OOM Killer の履歴",
			"journalctl -k を実行できませんでした（"+probeFailure(res, err)+"）。"))
	}

	hits := collectOOM(res.Stdout, in.Runners)
	if len(hits) == 0 {
		return one(check.Of(c, check.Result{
			Status:  check.OK,
			Summary: "OOM Killer の履歴なし",
			Detail:  "直近 24 時間のカーネルログに OOM Killer の記録はありません。",
		}))
	}
	return c.report(hits, in.Runners)
}

// report は対象ごとの痕跡を行にする。
//
// 行を対象ごとに分けるのは、どの runner が落ちているかが対処（並列度の削減か
// 増設か）の判断材料になるためである。
func (c oomCheck) report(hits map[string]*oomHit, runners []runner.Runner) []check.Result {
	out := make([]check.Result, 0, len(hits))
	for _, name := range hitOrder(hits, runners) {
		h := hits[name]
		where := "ホスト全体"
		if name != "" {
			where = name
		}
		out = append(out, check.Of(c, check.Result{
			Target:  name,
			Status:  check.Warn,
			Summary: "OOM による停止履歴あり",
			Detail: where + " に関係する OOM Killer の記録が直近 24 時間で " +
				strconv.Itoa(h.count) + " 件あります。最新: " + truncate(h.last, detailLineLimit),
			Impact: impactOOM,
			Remedy: "メモリの増設、ジョブの並列度の削減、swap の追加のいずれかを検討してください。",
		}))
	}
	return out
}

// oomHit は 1 つの対象に紐付いた痕跡。
type oomHit struct {
	count int
	last  string
}

// collectOOM はカーネルログから OOM の行を拾い、対象ごとにまとめる。
//
// 対象の推定は runner 名と runner プロセスの名前で行う。どれにも当たらない行は
// 空のキー（ホスト全体）へ入れる。**取りこぼしても件数から消さない**——他の
// プロセスが落ちていることもメモリ不足の証拠だからである。
func collectOOM(stdout []byte, runners []runner.Runner) map[string]*oomHit {
	hits := make(map[string]*oomHit)
	for line := range strings.SplitSeq(string(stdout), "\n") {
		if !hasMarker(line) {
			continue
		}
		key := attribute(line, runners)
		h, ok := hits[key]
		if !ok {
			h = &oomHit{count: 0, last: ""}
			hits[key] = h
		}
		h.count++
		h.last = strings.TrimSpace(line)
	}
	return hits
}

// hasMarker は行が OOM Killer の痕跡かを返す。
func hasMarker(line string) bool {
	for _, m := range oomMarkers {
		if strings.Contains(line, m) {
			return true
		}
	}
	return false
}

// attribute は行を runner に結び付ける。結び付かなければ空を返す。
func attribute(line string, runners []runner.Runner) string {
	for _, r := range runners {
		if name := r.Name(); name != "" && strings.Contains(line, name) {
			return name
		}
	}
	if strings.Contains(line, "Runner.Listener") || strings.Contains(line, "Runner.Worker") {
		// runner のプロセスだと分かるが、どの runner かまでは分からない。
		return ""
	}
	return ""
}

// hitOrder は行の並びを固定する。runner の検出順に並べ、ホスト全体を最後に置く。
func hitOrder(hits map[string]*oomHit, runners []runner.Runner) []string {
	out := make([]string, 0, len(hits))
	for _, r := range runners {
		if name := r.Name(); hits[name] != nil {
			out = append(out, name)
		}
	}
	if hits[""] != nil {
		out = append(out, "")
	}
	return out
}
