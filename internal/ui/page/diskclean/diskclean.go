// Package diskclean は Disk タブのクリーンアップ実行（FR-30）を担う。
//
// Disk タブ（page/disk）から分けているのは、**実行が集計・表示と独立している**
// ためである。入力は承認済みの計画（disk.CleanPlan）1 つだけで、表の行にも選択にも
// 触れない。1 ディレクトリ 2000 行の上限（atomic-design.md「ディレクトリの行数」）
// に対しても、Disk タブで切れる境界はここしか無い。
//
// **置き場所は基準どおりではない。** 利用者は 1 タブだけなので既定ではネスト
// （page/<tab>/<名前>）だが、切り出しを指示した Issue が `shared` への登録を受け入れ
// 条件に含めていたため page/ 直下にある（atomic-design.md「`page/` は 1 ディレクトリ
// 1 タブではない」の例外）。**次にタブ 1 枚ぶんを切り出すときはネスト側に倣うこと。**
//
// **確認を経ない破壊的経路を作らない不変条件はタブ側に残る**
// （security.md「確認を経ない破壊的経路を作らない」）。ここは承認済みの計画を受け
// 取って走らせるだけで、確認ダイアログを開く責務を持たない。Start を呼ぶのは
// dialog.DecidedMsg{Confirmed: true} を受けた 1 か所だけである。
//
// 監査ログもここでは扱わない。docker system prune -f の記録は Executor の手前
// （internal/exec/command）で、ファイル削除の記録は internal/disk の removeTarget
// 1 か所で行う（Issue #71 / security.md の監査ログ）。運ぶのは記録先だけである。
package diskclean

import (
	"context"
	"strconv"

	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/audit"
	"github.com/ousiassllc/gsr-helper/internal/disk"
	"github.com/ousiassllc/gsr-helper/internal/exec"
	"github.com/ousiassllc/gsr-helper/internal/ui/atom"
	"github.com/ousiassllc/gsr-helper/internal/ui/molecule"
	"github.com/ousiassllc/gsr-helper/internal/ui/organism/pane"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/disk/cleanview"
)

// ProgressMsg は進捗 1 件の到着。OK が偽なら channel が閉じたことを表す。
type ProgressMsg struct {
	Progress disk.Progress
	OK       bool
}

// DoneMsg はクリーンアップの終了。
//
// **失敗件数は載せない。** 件数は行の状態から数える（cleanview.Counts）ので出どころは
// 1 つである。実行側の件数も受け取ると、到着順によって 2 つの数え方が違う答えを出す。
// 到着順そのものへの対処は Settled が受け持つ。
type DoneMsg struct {
	Err error
}

// Job は実行中のクリーンアップ。
//
// **世代を持たない。** 集計と違い、クリーンアップは同時に 1 本しか走らない（実行中の
// c はタブ側が弾き、確認ダイアログも開かない）。畳んだ後に届いた結果は「Job が無い」
// ことで捨てられるので、世代を突き合わせる相手がそもそも無い。
type Job struct {
	// cancel は実行の打ち切り。終了時に呼んで context を解放する。
	cancel context.CancelFunc
	// ch は進捗が流れてくる channel。
	ch <-chan disk.Progress
	// doneCh は終了通知が流れてくる channel。
	doneCh <-chan DoneMsg
	// done / total は進捗表示の分母と分子（pane.ProgressInput）。
	done  int
	total int
	// bytes は解放見込み。結果報告に使う。
	bytes int64
	// rows は対象ごとの進み具合。ProgressList へそのまま渡す。
	rows []molecule.ProgressView
	// report は完了後の結果報告。実行中は空。
	report []string
	// closed は進捗の channel が閉じたか、result は実行の終了通知。
	//
	// **両方そろうまで結果を確定しない**（Settled）。進捗を待つ Cmd と終了を待つ Cmd は
	// 別の goroutine で走るため到着順が決まっておらず、終了が先に届いた回だけ
	// 最後の対象が未着手のまま報告に載る。
	closed bool
	result *DoneMsg
}

// Start は削除計画を実行し、Job と進捗・終了を待つ Cmd を返す。
//
// 進捗の channel を対象数ぶん buffer するのは、UI が受け取る前に実行側が止まらない
// ようにするためである。**削除の途中で描画待ちになる形にしてはいけない**（利用者が
// タブを切り替えただけで削除が止まりうる）。buffer があるので disk.Apply は最後まで
// 走り切り、UI は自分のペースで進捗を拾える。
//
// **件数はここでは数えない。** 出どころは進捗の行の状態 1 つに固定してある
// （cleanview.Counts）。実行側でも数えると、進捗と終了通知の到着順によって 2 つの
// 数え方が違う答えを出す。
func Start(tab int, ex exec.Executor, lg *audit.Logger, plan disk.CleanPlan) (*Job, tea.Cmd) {
	total := len(plan.Paths)
	if plan.Docker {
		total++
	}

	ctx, cancel := context.WithCancel(context.Background())
	ch := make(chan disk.Progress, total+1)
	doneCh := make(chan DoneMsg, 1)

	go func() {
		err := disk.Apply(ctx, ex, lg, plan, func(p disk.Progress) { ch <- p })
		close(ch)
		doneCh <- DoneMsg{Err: err}
	}()

	j := &Job{
		cancel: cancel, ch: ch, doneCh: doneCh, done: 0, total: total,
		bytes: plan.Bytes, rows: cleanview.Rows(plan), report: nil,
		closed: false, result: nil,
	}
	return j, tea.Batch(j.Next(tab), j.waitDone(tab))
}

// Stop は実行を打ち切る。
//
// **打ち切りは対象の途中にも効く。** disk.Apply は対象と対象の間で ctx.Err() を見る
// が、1 対象を消す removeTree も再帰の各段の入口で ctx.Err() を見るため（そもそも
// os.RemoveAll を使わずに自前で再帰しているのは、キャンセルを途中で受け取れるように
// するためである）、打ち切った瞬間に消えかけのツリーが残りうる。途中で切られた対象は
// 失敗として監査ログに 1 レコード残る（docs/architecture/data-model.md /
// docs/architecture/security.md）。
//
// そのうえで呼ぶのは終了時（page.ShutdownMsg）だけである。裏へ回っただけで止めないの
// は、消えかけのツリーが残るという後始末の要る状態を、利用者がタブを切り替えただけの
// つもりでいる間に作らないためである。終了は利用者が明示的に選んだ操作なので、そこで
// 打ち切ってプロセスを終わらせるほうが、消えかけのツリーを残したまま裏で削除を続ける
// よりも筋が通る。
func (j *Job) Stop() { j.cancel() }

// Next は進捗 1 件の到着を待つ Cmd を返す（集計の waitUsage と同じ形）。
func (j *Job) Next(tab int) tea.Cmd {
	ch := j.ch
	return page.Do(tab, func() tea.Msg {
		p, ok := <-ch
		return ProgressMsg{Progress: p, OK: ok}
	})
}

// waitDone は終了を待つ Cmd を返す。
func (j *Job) waitDone(tab int) tea.Cmd {
	ch := j.doneCh
	return page.Do(tab, func() tea.Msg { return <-ch })
}

// Mark は進捗 1 件を行へ反映する。OK が偽なら「全対象を送り終えた」として控える。
func (j *Job) Mark(msg ProgressMsg) {
	if !msg.OK {
		j.closed = true
		return
	}
	j.done = msg.Progress.Done
	cleanview.Mark(j.rows, msg.Progress)
}

// Record は終了通知を控える。
func (j *Job) Record(msg DoneMsg) { j.result = &msg }

// Settled は進捗を出し切ったことと終了通知の両方がそろったかを返す。
//
// **片方だけでは確定しない。** 2 つは別の goroutine から届き、到着順が決まっていない。
// 終了が先に届いた時点で数えると、最後の対象がまだ未着手のまま報告に載り、
// 進捗表示の「未実行 1 件」と状態行の「N 件を解放しました」が食い違う。
func (j *Job) Settled() bool { return j.closed && j.result != nil }

// Input は進捗表示へ送る中身を組み立てる。
func (j *Job) Input() pane.ProgressInput {
	return pane.ProgressInput{
		Title:  cleanview.ProgressTitle,
		Rows:   j.rows,
		Done:   j.done,
		Total:  j.total,
		Report: j.report,
	}
}

// Settle は結果を確定し、進捗表示へ送る中身と状態行の 1 行を返す。
// 確定できる状態でなければ第 3 戻り値に偽を返し、**何もしない。**
//
// 実行の context は確定したときにだけ解放するので、**真を返した後の Job は使わない。**
//
// **両方の合図がそろっていない呼び出しをここで弾くのが要点である。** 判定
// （Settled）を呼び出し側の作法に委ねると、終了通知だけが届いた時点で報告を組める
// 形が残り、最後の対象が未着手のまま「未実行 1 件」として数えられる。切り出す前は
// 同じガードが確定処理と同じ関数の中にあった（page/disk の finish）ので、構造で
// 守られていた性質である。
//
// **ここが守るのは件数の不変条件だけである。** 呼び出し側の `!ok` は依然として要る
// ——外すと進捗表示へゼロ値の中身が送られ、状態行が空になり、確定しなかった Job の
// context が解放されないまま捨てられる。
//
// **件数の出どころは行の状態 1 つに固定する**（cleanview.Counts）。報告は
// ProgressList の結果報告欄に出し、状態行にも 1 行残す。進捗表示を閉じたあとでも
// 結果が読めるようにするためである（次の打鍵で消える）。
func (j *Job) Settle() (pane.ProgressInput, string, bool) {
	if !j.Settled() {
		return pane.ProgressInput{}, "", false
	}

	err := j.result.Err
	j.report = cleanview.Report(j.rows, j.bytes, err)
	in := j.Input()
	j.cancel()
	return in, notice(j.rows, j.bytes, err), true
}

// notice は結果報告の 1 行を返す。
//
// 失敗があるときに解放量を出さないのは、消せなかった対象のぶんが含まれた見込み値に
// なるためである。見込みと実測が食い違う数字を「解放しました」と書くと、次に何をす
// べきかの判断を誤らせる。
func notice(rows []molecule.ProgressView, bytes int64, err error) string {
	done, failed, pending := cleanview.Counts(rows)
	switch {
	// **未実行が残っているかを先に見る。** disk.Apply は打ち切りでも errors.Join で
	// エラーを返すので、err の有無で先に分岐すると「中断」が永久に出ない（そのうえ
	// 「失敗しました: クリーンアップを中断しました: context canceled」という二重の
	// 前置きと生の Go エラー文字列が状態行に出る）。
	case pending > 0:
		return "クリーンアップを中断しました: " + strconv.Itoa(done) + " 件完了 / " +
			strconv.Itoa(failed) + " 件失敗 / " + strconv.Itoa(pending) + " 件未実行"
	case failed > 0:
		return "クリーンアップ完了: " + strconv.Itoa(done) + " 件成功 / " +
			strconv.Itoa(failed) + " 件失敗"
	case err != nil:
		return "クリーンアップに失敗しました: " + cleanview.FirstLine(err.Error())
	default:
		return "クリーンアップ完了: " + strconv.Itoa(done) + " 件 / " + atom.Bytes(bytes) + " を解放しました"
	}
}
