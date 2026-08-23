// Package workscan は runner ごとの _work 使用量の集計を親 Model の代わりに駆動する
// （Issue #73）。
//
// 親 Model（ui.App）から分けているのは 2 つの理由による。
//   - 集計はディレクトリの再帰走査という副作用であり、タブの配送と共有状態の
//     組み立てを持つ親 Model とは独立した責務である。
//   - internal/ui 直下は 1 ディレクトリ 2000 行の上限に対して余裕が無く、
//     取得処理を置くと親 Model 自身のテストを足す余地が先に尽きる
//     （internal/ui/hostreq と同じ判断）。
//
// **UI ランタイムを知らない。** tea.Cmd を返す以外に bubbletea の状態を持たず、
// 集計を張る・畳むの判断は親が行う。
//
// **この集計は 3 秒ごとの再検出サイクルには載せない。** 走査は runner 1 台で
// 秒〜分かかりうるため、既定タブ（Runners）の更新周期に載せると
// 「起動から一覧表示まで 1 秒以内」（docs/requirements/non-functional.md）を壊す。
// 駆動するのは最初の検出が成功した直後と、手動の再読み込み（r）のときだけである
// （親の startWorkScan）。
//
// Disk タブが自分で駆動する内訳の集計（page/disk の scan.go）とは別物である。
// あちらは削除の単位ごとの内訳を判明順に出すためのもので、こちらは 1 台につき
// 合計 1 つだけを返す。
package workscan

import (
	"context"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/disk"
	"github.com/ousiassllc/gsr-helper/internal/runner"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
)

// Budget は 1 周期の集計全体に与える上限。
//
// 検出（discovery.Budget = 15 秒）より大幅に長く取るのは、走査するのが
// systemctl の応答ではなくファイルシステムだからである。ジョブが数十万ファイルを
// 展開した _work では 1 台で分単位かかりうる。
//
// それでも上限を置くのは、期限が無いと NFS の応答待ちなどで goroutine が
// プロセスの寿命ぶん残り、手動の再読み込みのたびに積み上がるためである。
// 期限切れの runner は集計失敗として扱い、一覧では `-` に縮退する。
const Budget = 3 * time.Minute

// Msg は集計 1 周期分の結果。親は Seq で古い周期の結果を捨てる。
//
// 1 台ずつではなく全台まとまってから返すのは、_WORK 列が「埋まっていく」表示を
// 必要としないためである（Disk タブは判明順に埋めるが、あちらは対象を選ばせる
// 画面で、判明した行から操作できることに意味がある）。まとめて返すぶん、
// 親は共有状態の配布を 1 度で済ませられる。
type Msg struct {
	// Seq は何周期目かの通し番号。手動の再読み込みで新しい周期が始まったあとに
	// 前の周期が返ることがあるため、親が突き合わせて捨てる。
	Seq int
	// Usage は runner ディレクトリごとの使用量。キーは runner.Runner.Dir。
	Usage map[string]page.WorkUsage
}

// Start は runners の _work 使用量を並行に集計する Cmd を返す。
//
// runner ごとに goroutine を分けるのは、巨大な _work を持つ 1 台が他の台の結果を
// 待たせないようにするためである。台数はホスト上の runner 数（多くても数十）に
// 限られるので、並行数に上限は置かない。
//
// 1 台の失敗で周期全体を落とさない。失敗は page.WorkUsage.Err として台ごとに
// 載せ、その台だけが `-` に縮退する。
func Start(seq int, runners []runner.Runner) tea.Cmd {
	if len(runners) == 0 {
		return nil
	}

	// Cmd は別 goroutine で走るため、入力は写し取ってから渡す（親の状態が
	// 途中で書き換わっても集計の入力が変わらないようにする）。
	targets := make([]runner.Runner, len(runners))
	copy(targets, runners)

	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), Budget)
		defer cancel()

		var mu sync.Mutex
		usage := make(map[string]page.WorkUsage, len(targets))

		var wg sync.WaitGroup
		for _, r := range targets {
			wg.Add(1)
			go func() {
				defer wg.Done()

				bytes, err := disk.WorkUsage(ctx, r)
				mu.Lock()
				defer mu.Unlock()
				usage[r.Dir] = page.WorkUsage{Bytes: bytes, Err: err}
			}()
		}
		wg.Wait()
		return Msg{Seq: seq, Usage: usage}
	}
}

// State は集計の進行状況。親 Model はこれを 1 つ持ち、周期の通し番号と実行中かの
// 管理をこちらへ預ける。
//
// **親に int と bool を並べさせない。** 「古い周期を捨てる」「実行中は重ねない」は
// 集計の側の不変条件であり、持ち主が離れると片方だけを更新する経路ができる。
type State struct {
	seq   int
	busy  bool
	usage map[string]page.WorkUsage
}

// Usage は runner ディレクトリごとの使用量を返す。**キーが無いことが未集計を表す**
// （page.DiskState.Work の doc）。
func (s *State) Usage() map[string]page.WorkUsage { return s.usage }

// Start は集計を始める Cmd を返す。始めなかった場合は nil を返す。
//
// **実行中は重ねない。** 手動の再読み込み（r）を連打すると、巨大な _work を走査する
// goroutine が押した回数だけ積み上がる。次の契機で始め直せるので重ねない側に倒す。
//
// 未着手かどうかは Started で見る。runner が 0 台のときは周期を進めず、次の契機へ譲る。
func (s *State) Start(runners []runner.Runner) tea.Cmd {
	if s.busy {
		return nil
	}
	cmd := Start(s.seq+1, runners)
	if cmd == nil {
		return nil
	}
	s.seq++
	s.busy = true
	return cmd
}

// Started は 1 度でも集計を始めたかを返す。
func (s *State) Started() bool { return s.seq > 0 }

// Apply は集計 1 周期分の結果を取り込む。
//
// 古い周期の結果は捨てる。再読み込みで新しい周期が始まったあとに前の周期が返ると、
// 消えた対象の使用量が新しい一覧へ混ざる。
func (s *State) Apply(msg Msg) {
	if msg.Seq != s.seq {
		return
	}
	s.busy = false
	s.usage = msg.Usage
}
