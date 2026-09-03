// Package discovery は runner の検出と、その周期の管理を親 Model の代わりに担う。
//
// 親 Model（ui）から切り出してあるのは 2 つの理由による。1 つは行数で、ui 直下は
// 1 ディレクトリ 2000 行の上限に対して余裕が無く、タブが要する起動時の値を足すたびに
// 押し上がる（internal/ui/hostreq と同じ事情）。もう 1 つは境界で、実行中の本数
// （inflight）と通し番号（seq）をどう進めるか・追い抜かれた周期をどう捨てるかは
// 検出の側の不変条件であり、親に int を並べさせると片方だけを更新する経路ができる
// （workscan / ghscope と同じ判断。Issue #139）。
//
// **UI ランタイムを知らない。** tea.Cmd と TickMsg を返す以外に bubbletea の状態を
// 持たず、周期を始める契機（起動・Tick・手動の再読み込み）を決めるのは親である。
// 親が渡す 1 周期分の入力は Input にまとめてあり、appconfig の型は持ち込まない
// （設定とフラグの合成・既定値の解決は親の仕事。Input の doc）。
package discovery

import (
	"context"
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/exec"
	"github.com/ousiassllc/gsr-helper/internal/runner"
)

const (
	// DefaultRefresh は自動更新間隔の最終的な既定値（非機能要件の応答性・性能）。
	// 設定ファイルにもフラグにも値が無い場合に使う。
	DefaultRefresh = 3 * time.Second
	// MinRefresh は自動更新間隔の下限。1 秒より短い間隔は画面の更新として意味が無く
	// （人が読み取れない）、検出の Cmd を無駄に発行し続けるだけなので切り上げる。
	MinRefresh = time.Second
	// Budget は 1 回の検出に与える上限（deadline）。
	//
	// **自動更新間隔とは切り離す。** 間隔と同値にすると --refresh 1 のような短い間隔では
	// ほぼ毎周期で期限切れになり、runner.Discover が残りの systemctl show を発行せずに
	// 取れた分だけを返す。その部分結果では Svc が紐付かない runner が出るため、systemd
	// 管理の runner が run.sh / - と誤表示され、孤児ユニットも過少報告される。
	//
	// 値は 15 秒とする。runner.Discover は systemctl show を 8 件ずつのバッチで発行し、
	// 1 コマンドの上限は exec の既定（30 秒）である。正常時の show は数ミリ秒で終わるので、
	// 想定台数（20 台程度）でも 15 秒は十分に余る。一方 systemctl が応答しない異常時には
	// 1 コマンドの上限（30 秒）より先に打ち切るため、1 回の検出が 30 秒以上生き残って
	// 積み上がることはない。期限切れの周期の部分結果は採用せず、直前の成功結果を保つ
	// （State.Apply が呼ぶ Reconcile の分岐）。
	Budget = 15 * time.Second
)

// Msg は検出の結果。
//
// Err は「検出そのものが時間内に終わらなかった」ことを表す。1 件ごとの部分的な
// 失敗は runner.Result.Warnings に集約されるため、ここには載せない。
//
// Seq は何回目の検出かの通し番号。**古い周期の結果で新しい結果を上書きしないため
// に必要である。** 検出には最大 Budget（15 秒）かかるので、遅い周期が
// 新しい周期より後に返ることがあり、番号が無いと一覧が古い内容へ巻き戻る。
type Msg struct {
	Seq    int
	Result runner.Result
	Err    error
}

// Start は runner を検出する Cmd を返す。
//
// ドメイン層の呼び出しは Cmd の中だけで行い、Update の中では行わない。検出は
// ディスク走査・/proc 走査・systemctl 参照を伴い、UI スレッドで走らせるとキー入力への
// 反応が止まるためである（非機能要件の「UI をブロックしない処理」）。
//
// seq と opts は呼び出し側（State.Start）が Cmd の外で確定させて渡す。Cmd が
// 別の goroutine で走る間に親 Model の状態が書き換わっても、検出の入力が変わらない
// ようにするためである。実行中の本数（inflight）を進めて二重起動を防ぐのも
// 呼び出し側の責務であり、この関数では数えない（State.Start を参照）。
func Start(seq int, opts runner.Options) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), Budget)
		defer cancel()

		res := runner.Discover(ctx, opts)
		return Msg{Seq: seq, Result: res, Err: timeoutErr(ctx, Budget)}
	}
}

// Exec は検出に使う Executor を返す。
//
// systemctl が無い環境（systemd が偽）では nil を返す。runner.Discover は
// Executor が nil のとき systemd を参照しない（systemctl 不在時の縮退）ため、
// 3 秒ごとに失敗するコマンドを発行し続けずに済む。
func Exec(systemd bool, ex exec.Executor) exec.Executor {
	if !systemd {
		return nil
	}
	return ex
}

// timeoutErr は検出が期限内に終わらなかった場合のエラーを返す。
func timeoutErr(ctx context.Context, timeout time.Duration) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("検出が %s 以内に終わりませんでした: %w", timeout, err)
	}
	return nil
}
