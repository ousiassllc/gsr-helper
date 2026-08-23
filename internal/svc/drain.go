package svc

import (
	"context"
	"fmt"
	"time"

	"github.com/ousiassllc/gsr-helper/internal/exec"
	"github.com/ousiassllc/gsr-helper/internal/runner"
	"github.com/ousiassllc/gsr-helper/internal/runner/procs"
)

// defaultInterval は Runner.Worker が消えたかを確かめる既定の間隔。
//
// 一覧の自動更新（3 秒）より短くしているのは、検知が遅れた分だけ停止も遅れるためである。
// 1 回あたりの負荷は /proc の走査 1 回ぶんで、systemd への問い合わせは伴わない。
const defaultInterval = 2 * time.Second

// Progress はドレイン待機の途中経過。
//
// docs/ui/screens.md の「ドレイン待機中」が出す経過時間と対象ジョブに対応する。
// Worker ごとの経過時間は procs.Process.Elapsed から取る（Runner.JobElapsed は最も
// 古い 1 件しか返さないため、Worker ごとに 1 行出す表示は作れない）。
type Progress struct {
	// Elapsed は待機を始めてからの経過時間。
	Elapsed time.Duration
	// Workers はまだ残っている Runner.Worker。空なら待機は終わっている。
	Workers []procs.Process
}

// Drainer はドレイン停止の実行者。
//
// ポーリングの手段（Scan）・間隔（Interval）・時刻（Now）を差し替えられるようにして
// いるのは、待機そのものを実時間と実 /proc に依らず検証するためである。待ち時間が
// 無制限（FR-07）である以上、既定値のまま試すテストは終わらないかホストの状態に
// 依存する。差し替えが要るのはテストだけなので、本番の呼び出しは Drain を使う。
type Drainer struct {
	// Exec は停止コマンドの発行に使う Executor。
	Exec exec.Executor
	// Scan は稼働プロセスの取得。nil なら procs.Scan を使う。
	Scan func() ([]procs.Process, error)
	// Interval はポーリングの間隔。0 以下なら defaultInterval を使う。
	Interval time.Duration
	// Now は現在時刻。nil なら time.Now を使う。
	Now func() time.Time
}

// Drain は runner の Runner.Worker が消滅するのを待ってから停止する（FR-07）。
//
// **待ち時間は無制限である。** 打ち切りの上限を持たせると、長いジョブの途中で
// 「ドレイン停止したはずが強制停止になっていた」という結果になる。中断は ctx の
// キャンセルだけで行い、そのときは ctx.Err() を返して**停止処理は行わない**
// （待機を取り消したのであって、停止を指示されたわけではないため）。
//
// progress は待機開始直後と各ポーリングごとに呼ぶ。nil なら呼ばない。最初の走査で
// 既に Worker が 0 件なら、間隔を待たずにそのまま停止へ進む。
//
// **新しいジョブを受け付けないことは保証しない。** GitHub に受付停止の API が無いため、
// 待機中に Runner.Listener が次のジョブを拾いうる（docs/requirements/functional.md の
// 「ドレイン停止の制約」）。この制約は待機画面にも明示する。
//
// **ユニット名が無ければ待機に入らず ErrNoUnit を返す。** 待ち切った先で発行するのは
// systemctl stop であり、ユニット名が分からなければその 1 本は必ず失敗する
// （unitCommand）。無制限に待つ処理が「待ち切っても必ず失敗する」と分かっている状態で
// 待ち始めてはならない。利用者はジョブの完了まで待たされた末に、最初から分かっていた
// 理由で失敗を告げられることになる。
func (d Drainer) Drain(ctx context.Context, r runner.Runner, progress func(Progress)) error {
	if r.UnitName == "" {
		return fmt.Errorf("%s: %w", r.Name(), ErrNoUnit)
	}

	scan := d.Scan
	if scan == nil {
		scan = procs.Scan
	}
	interval := d.Interval
	if interval <= 0 {
		interval = defaultInterval
	}
	now := d.Now
	if now == nil {
		now = time.Now
	}

	start := now()
	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		all, err := scan()
		if err != nil {
			return fmt.Errorf("%s の稼働プロセスを走査できませんでした: %w", r.Name(), err)
		}

		workers := workersOf(r, all)
		if progress != nil {
			progress(Progress{Elapsed: now().Sub(start), Workers: workers})
		}
		if len(workers) == 0 {
			return Stop(ctx, d.Exec, r)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(interval):
		}
	}
}

// Drain は既定の設定でドレイン停止する。走査は procs.Scan、間隔は 2 秒である。
// 詳細は Drainer.Drain を参照。
func Drain(ctx context.Context, ex exec.Executor, r runner.Runner, progress func(Progress)) error {
	return Drainer{Exec: ex}.Drain(ctx, r, progress)
}

// workersOf は r に属する Runner.Worker だけを返す。
//
// procs.Scan はホスト全体の runner プロセスを返すため、ディレクトリで絞る。
// Runner.Dir と procs.Process.Dir はどちらもシンボリックリンクを解決した絶対パスなので、
// 文字列の一致で照合できる。**Runner.Workers は使わない。** あれは検出時点の
// スナップショットであり、待機中に更新されないためである。
func workersOf(r runner.Runner, all []procs.Process) []procs.Process {
	out := make([]procs.Process, 0, len(all))
	for _, p := range all {
		if p.Kind == procs.Worker && p.Dir == r.Dir {
			out = append(out, p)
		}
	}
	return out
}
