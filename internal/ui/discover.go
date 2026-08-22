package ui

import (
	"context"
	"fmt"
	"slices"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/exec"
	"github.com/ousiassllc/gsr-helper/internal/runner"
)

const (
	// defaultRefresh は自動更新間隔の最終的な既定値（非機能要件の応答性・性能）。
	// 設定ファイルにもフラグにも値が無い場合に使う。
	defaultRefresh = 3 * time.Second
	// minRefresh は自動更新間隔の下限。1 秒より短い間隔は画面の更新として意味が無く
	// （人が読み取れない）、検出の Cmd を無駄に発行し続けるだけなので切り上げる。
	minRefresh = time.Second
	// discoverBudget は 1 回の検出に与える上限（deadline）。
	//
	// **自動更新間隔とは切り離す。** 間隔と同値にすると --refresh 1 のような短い間隔では
	// ほぼ毎周期で期限切れになり、runner.ScanUnits が残りの systemctl show を発行せずに
	// 取れた分だけを返す。その部分結果では Svc が紐付かない runner が出るため、systemd
	// 管理の runner が run.sh / - と誤表示され、孤児ユニットも過少報告される。
	//
	// 値は 15 秒とする。runner.ScanUnits は systemctl show を 8 件ずつのバッチで発行し、
	// 1 コマンドの上限は exec の既定（30 秒）である。正常時の show は数ミリ秒で終わるので、
	// 想定台数（20 台程度）でも 15 秒は十分に余る。一方 systemctl が応答しない異常時には
	// 1 コマンドの上限（30 秒）より先に打ち切るため、1 回の検出が 30 秒以上生き残って
	// 積み上がることはない。期限切れの周期の部分結果は採用せず、直前の成功結果を保つ
	// （app.go の discoveredMsg の分岐）。
	discoverBudget = 15 * time.Second
)

// tickMsg は自動更新の契機。非公開にして、外から自動更新を駆動できないようにする。
type tickMsg struct{}

// discoveredMsg は検出の結果。
//
// err は「検出そのものが時間内に終わらなかった」ことを表す。1 件ごとの部分的な
// 失敗は runner.Result.Warnings に集約されるため、ここには載せない。
//
// seq は何回目の検出かの通し番号。**古い周期の結果で新しい結果を上書きしないため
// に必要である。** 検出には最大 discoverBudget（15 秒）かかるので、遅い周期が
// 新しい周期より後に返ることがあり、番号が無いと一覧が古い内容へ巻き戻る。
type discoveredMsg struct {
	seq    int
	result runner.Result
	err    error
}

// tick は次の自動更新を予約する Cmd を返す。
func (a App) tick() tea.Cmd {
	return tea.Tick(a.refresh(), func(time.Time) tea.Msg { return tickMsg{} })
}

// firstTick は待たずに tickMsg を返す Cmd。Init が最初の周期を始めるのに使う。
//
// Init から直に検出を始めない理由は Init の doc を参照。
func firstTick() tea.Cmd {
	return func() tea.Msg { return tickMsg{} }
}

// onTick は自動更新 1 周期分の Cmd を返す。
//
// **実行中の検出があるときは新しい検出を始めない。** 自動更新間隔（既定 3 秒、下限
// 1 秒）は 1 回の検出に許す時間（discoverBudget = 15 秒）より短いため、無条件に
// 発行すると最大 5 周期が重なる。1 回の検出は systemctl show を showConcurrency（8）
// 並列で撒くので、systemctl が応答しない——まさに検出が長引くその状況で——最大
// 40 プロセスが同時に走ることになる。次の周期は必ず予約するので、遅い検出が
// 終わればそのまま自動更新は続く。
func (a *App) onTick() tea.Cmd {
	if a.inflight > 0 {
		return a.tick()
	}
	return tea.Batch(a.discover(), a.tick())
}

// applyDiscovered は検出結果を取り込み、共有状態を配る Cmd を返す。
func (a *App) applyDiscovered(msg discoveredMsg) tea.Cmd {
	if a.inflight > 0 {
		a.inflight--
	}
	if msg.seq < a.applied {
		// 追い抜かれた周期の結果は捨てる（discoveredMsg.seq の doc）。
		return nil
	}
	a.applied = msg.seq

	// 期限切れ・失敗した周期の部分結果では上書きしない。runner.ScanUnits は
	// ctx がキャンセルされた時点で残りの systemctl show を発行せず取れた分だけを
	// 返すため、部分結果を採ると systemd 管理の runner が run.sh / - と誤表示され、
	// 孤児ユニットも過少報告される。エラーは状態行の警告として出し、一覧は
	// 直前の成功結果を保つ。
	a.err = msg.err
	if msg.err == nil {
		a.result = msg.result
	}
	return a.distribute()
}

// discover は runner を検出する Cmd を返す。
//
// ドメイン層の呼び出しは Cmd の中だけで行い、Update の中では行わない。検出は
// ディスク走査・/proc 走査・systemctl 参照を伴い、UI スレッドで走らせるとキー入力への
// 反応が止まるためである（非機能要件の「UI をブロックしない処理」）。
//
// 引数に必要な値を Cmd の外で写し取るのは、Cmd が別 goroutine で走る間に親 Model の
// 状態が書き換わっても、検出の入力が変わらないようにするためである。
// 実行中の本数（inflight）と通し番号（seq）を進めるためポインタで受け取る。
// 発行した検出を親が数えられないと二重起動を防げない（onTick を参照）。
func (a *App) discover() tea.Cmd {
	a.inflight++
	a.seq++
	seq := a.seq
	ex := a.discoverExec()
	roots := slices.Clone(a.opts.Roots)
	depth := a.cfg.ScanDepth

	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), discoverBudget)
		defer cancel()

		res := runner.Discover(ctx, runner.Options{Roots: roots, Depth: depth, Exec: ex})
		return discoveredMsg{seq: seq, result: res, err: timeoutErr(ctx, discoverBudget)}
	}
}

// discoverExec は検出に使う Executor を返す。
//
// systemctl が無い環境では nil を返す。runner.ScanUnits は Executor が nil のとき
// systemd を参照しない（systemctl 不在時の縮退）ため、3 秒ごとに失敗するコマンドを
// 発行し続けずに済む。
func (a App) discoverExec() exec.Executor {
	if !a.caps.Systemd {
		return nil
	}
	return a.ex
}

// refresh は自動更新間隔を決める。フラグ → 設定ファイル → 既定値の順に採る。
//
// 検出の deadline には使わない（discoverBudget を参照）。表示を更新する間隔と、
// 1 回の検出に許す時間は別の関心事である。
func (a App) refresh() time.Duration {
	d := a.opts.Refresh
	if d <= 0 {
		d = a.cfg.RefreshDuration()
	}
	if d <= 0 {
		d = defaultRefresh
	}
	return max(d, minRefresh)
}

// timeoutErr は検出が期限内に終わらなかった場合のエラーを返す。
func timeoutErr(ctx context.Context, timeout time.Duration) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("検出が %s 以内に終わりませんでした: %w", timeout, err)
	}
	return nil
}
