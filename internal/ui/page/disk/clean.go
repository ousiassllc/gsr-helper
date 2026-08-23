package disk

import (
	"context"
	"strconv"

	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/disk"
	"github.com/ousiassllc/gsr-helper/internal/ui/atom"
	"github.com/ousiassllc/gsr-helper/internal/ui/molecule"
	"github.com/ousiassllc/gsr-helper/internal/ui/organism/dialog"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/disk/cleanview"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/disk/confirmmodal"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/progressmodal"
)

// クリーンアップ（FR-30）の一本道を実装する。
//
// **選択 → PlanClean（ドライラン）→ 確認 → Apply 以外の削除経路を作らない**
// （security.md「確認を経ない破壊的経路を作らない」）。この不変条件はコードの形で
// 守る。すなわち disk.Apply を呼ぶのは startClean 1 か所だけで、startClean を呼ぶのは
// dialog.DecidedMsg{Confirmed: true} を受けた onResult 1 か所だけである。c を押す
// requestClean は確認ダイアログを開く Cmd しか返さない。
//
// 監査ログはここでは扱わない。docker system prune -f の記録は Executor の手前
// （internal/exec/command）で、ファイル削除の記録は internal/disk の removeTarget
// 1 か所で行う（Issue #71 / security.md の監査ログ）。page がするのは記録先
// （m.st.Audit）を disk.Apply へ運ぶことだけであり、自分では 1 行も記録しない。
// page が自分で記録すると、別経路が増えたときに記録の抜けが page ごとに分かれる。

const (
	// noticeNoTarget は選択が空のまま c を押したときの案内。
	//
	// 黙って何もしないと「効かないキー」に見える（screens.md の設計原則 2）。
	noticeNoTarget = "対象が選択されていません"
	// noticeRunning はクリーンアップ中に c を押したときの案内。
	noticeRunning = "クリーンアップを実行中です"
	// noticeBecameBusy は承認を待つ間にジョブが始まったため中止したときの案内。
	noticeBecameBusy = "ジョブが開始したため中止しました: "
)

// cleanState は実行中のクリーンアップ。nil なら実行していない。
//
// **世代を持たない。** 集計と違い、クリーンアップは同時に 1 本しか走らない
// （実行中の c は noticeRunning で弾き、確認ダイアログも開かない）。畳んだ後に届いた
// 結果は m.clean == nil で捨てられるので、世代を突き合わせる相手がそもそも無い。
type cleanState struct {
	// cancel は実行の打ち切り。終了時に呼んで context を解放する。
	cancel context.CancelFunc
	// ch は進捗が流れてくる channel。
	ch <-chan disk.Progress
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
	// **両方そろうまで結果を確定しない**（finish）。進捗を待つ Cmd と終了を待つ Cmd は
	// 別の goroutine で走るため到着順が決まっておらず、終了が先に届いた回だけ
	// 最後の対象が未着手のまま報告に載る。
	closed bool
	result *applyDoneMsg
}

// progressMsg は進捗 1 件の到着。ok が偽なら channel が閉じたことを表す。
type progressMsg struct {
	progress disk.Progress
	ok       bool
}

// applyDoneMsg はクリーンアップの終了。
//
// **失敗件数は載せない。** 件数は行の状態から数える（cleanview.Counts）ので出どころは
// 1 つである。実行側の件数も受け取ると、到着順によって 2 つの数え方が違う答えを出す。
// 到着順そのものへの対処は finish が受け持つ。
type applyDoneMsg struct {
	err error
}

// requestClean は c（Keys.Disk.Clean）に対する処理。**確認ダイアログを開くだけ**で、
// ここから削除は始まらない（設計原則 3）。
//
// ドライランを先に通すのは、確認画面に出す内容と実際に消すものを一致させるためで
// ある。検証を通らない対象が 1 件でもあれば PlanClean は計画そのものを作らないので
// （disk.PlanClean の doc）、**確認へは進まず理由を状態行に出す。** 通らない対象を
// 抱えたまま確認を出すと、y を押した後に「一部だけ消えた」ことに気付けない。
func (m *Model) requestClean() tea.Cmd {
	if m.clean != nil {
		m.notice = noticeRunning
		return nil
	}

	checked := m.tbl.Checked()
	if len(checked) == 0 {
		m.notice = noticeNoTarget
		return nil
	}

	plan, err := disk.PlanClean(cleanview.Targets(checkedUsage(checked)))
	if err != nil {
		m.notice = err.Error()
		return nil
	}

	m.plan = plan
	return confirmmodal.Open(&m.overlay, cleanview.ConfirmInput(plan))
}

// onResult は確認ダイアログの決定を処理する。
//
// **実行へ進む唯一の分岐である。** Confirmed が真のときだけ startClean を呼ぶ。
// 偽（n / esc / enter）のときは計画を捨てて閉じるだけで、Executor もファイルシステムも
// 触らない。
func (m *Model) onResult(msg page.ResultMsg) tea.Cmd {
	decided, ok := msg.Msg.(dialog.DecidedMsg)
	if msg.Kind != confirmmodal.Kind || !ok {
		return nil
	}

	m.overlay.Close()
	plan := m.plan
	m.plan = emptyPlan()
	if !decided.Confirmed {
		return nil
	}
	// 承認を待つ間にジョブが始まっていないかを見直す（cleanview.Reprotected の doc）。
	if label := cleanview.Reprotected(plan, m.st.Result.Runners); label != "" {
		m.notice = noticeBecameBusy + label
		return nil
	}
	return m.startClean(plan)
}

// startClean は削除計画を実行し、進捗と終了を待つ Cmd を返す。
//
// 進捗の channel を対象数ぶん buffer するのは、UI が受け取る前に実行側が止まらない
// ようにするためである。**削除の途中で描画待ちになる形にしてはいけない**（利用者が
// タブを切り替えただけで削除が止まりうる）。buffer があるので Apply は最後まで走り切り、
// UI は自分のペースで進捗を拾える。
//
// 失敗件数は callback の中で数える。callback は Apply と同じ goroutine から同期的に
// 呼ばれるので、この数え上げは競合しない。
func (m *Model) startClean(plan disk.CleanPlan) tea.Cmd {
	total := len(plan.Paths)
	if plan.Docker {
		total++
	}

	ctx, cancel := context.WithCancel(context.Background())
	ch := make(chan disk.Progress, total+1)
	doneCh := make(chan applyDoneMsg, 1)
	ex := m.st.Exec
	lg := m.st.Audit

	go func() {
		err := disk.Apply(ctx, ex, lg, plan, func(p disk.Progress) { ch <- p })
		close(ch)
		doneCh <- applyDoneMsg{err: err}
	}()

	m.clean = &cleanState{
		cancel: cancel, ch: ch, done: 0, total: total, bytes: plan.Bytes,
		rows: cleanview.Rows(plan), report: nil, closed: false, result: nil,
	}
	m.notice = ""
	return tea.Batch(m.waitProgress(ch), m.waitApply(doneCh), m.openProgress())
}

// emptyPlan は「確認中の計画は無い」を表すゼロ値を返す。
func emptyPlan() disk.CleanPlan {
	return disk.CleanPlan{Paths: nil, Docker: false, Bytes: 0, Commands: nil}
}

// stopClean は実行中のクリーンアップを打ち切る。実行していなければ何もしない。
//
// 呼ぶのは終了時（page.ShutdownMsg）だけである。裏へ回っただけで止めないのは、
// 削除が中途半端に終わった状態を利用者の知らないところで作らないためである
// （disk.Apply は対象と対象の間でしか打ち切りを見ない）。
func (m *Model) stopClean() {
	if m.clean == nil {
		return
	}
	m.clean.cancel()
	m.clean = nil
}

// waitProgress は進捗 1 件の到着を待つ Cmd を返す（集計の waitUsage と同じ形）。
func (m Model) waitProgress(ch <-chan disk.Progress) tea.Cmd {
	return page.Do(m.tab, func() tea.Msg {
		p, ok := <-ch
		return progressMsg{progress: p, ok: ok}
	})
}

// waitApply は終了を待つ Cmd を返す。
func (m Model) waitApply(ch <-chan applyDoneMsg) tea.Cmd {
	return page.Do(m.tab, func() tea.Msg { return <-ch })
}

// onProgress は進捗を ProgressList へ反映し、次の 1 件を待つ Cmd を返す。
//
// 全体件数は確認を通した計画の時点で確定しているため、進捗バーが出る
// （atomic-design.md の「bubbles/progress を使う範囲」）。
func (m *Model) onProgress(msg progressMsg) tea.Cmd {
	if m.clean == nil {
		return nil
	}
	if !msg.ok {
		// channel が閉じた＝全対象を送り終えた。終了通知が既に届いていれば確定する。
		m.clean.closed = true
		return m.finish()
	}
	m.clean.done = msg.progress.Done
	cleanview.Mark(m.clean.rows, msg.progress)
	return tea.Batch(m.waitProgress(m.clean.ch), m.updateProgress())
}

// onApplyDone は結果を報告し、選択を解いて再集計する。
//
// 再集計するのは使用量が変わったためである。残った表をそのまま出すと、消えた対象が
// 容量を持ったまま並び、もう一度選んで消せてしまうように見える。選択を解くのは、
// 消えた対象の選択が識別子ごと残って次のクリーンアップに紛れ込まないようにするため
// である（table.SetItems は消えた行の選択を捨てるが、再集計が終わるまでの間は
// 古い行が残っている）。
//
// 裏へ回った後に終わった場合もここから張り直す。裏では畳むという原則からは外れるが、
// この集計は有限時間で必ず終わり、戻ったときに古い使用量を見せないほうが実害が
// 小さい（畳むために「今前面か」を持つと、状態が 1 つ増えて寿命の通知と二重管理になる）。
func (m *Model) onApplyDone(msg applyDoneMsg) tea.Cmd {
	if m.clean == nil {
		return nil
	}

	m.clean.result = &msg
	return m.finish()
}

// finish は進捗を出し切ったことと終了通知の両方がそろった時点で結果を確定する。
//
// **片方だけでは確定しない。** 2 つは別の goroutine から届き、到着順が決まっていない。
// 終了が先に届いた時点で数えると、最後の対象がまだ未着手のまま報告に載り、
// 進捗表示の「未実行 1 件」と状態行の「N 件を解放しました」が食い違う。
func (m *Model) finish() tea.Cmd {
	if !m.clean.closed || m.clean.result == nil {
		return nil
	}

	// 報告は ProgressList の結果報告欄に出す。状態行にも 1 行残すのは、進捗表示を
	// 閉じたあとでも結果が読めるようにするためである（次の打鍵で消える）。
	// **件数の出どころは行の状態 1 つに固定する**（cleanview.Counts）。
	err := m.clean.result.err
	m.clean.report = cleanview.Report(m.clean.rows, m.clean.bytes, err)
	m.notice = cleanNotice(m.clean.rows, m.clean.bytes, err)
	report := m.updateProgress()
	stop := progressmodal.Stop(&m.overlay)

	m.clean.cancel()
	m.clean = nil
	m.tbl.ClearSelection()
	return tea.Batch(report, stop, m.startScan())
}

// cleanNotice は結果報告の 1 行を返す。
//
// 失敗があるときに解放量を出さないのは、消せなかった対象のぶんが含まれた見込み値に
// なるためである。見込みと実測が食い違う数字を「解放しました」と書くと、次に何をす
// べきかの判断を誤らせる。
func cleanNotice(rows []molecule.ProgressView, bytes int64, err error) string {
	done, failed, pending := cleanview.Counts(rows)
	switch {
	case failed > 0:
		return "クリーンアップ完了: " + strconv.Itoa(done) + " 件成功 / " +
			strconv.Itoa(failed) + " 件失敗"
	case err != nil:
		return "クリーンアップに失敗しました: " + cleanview.FirstLine(err.Error())
	case pending > 0:
		return "クリーンアップを中断しました: " + strconv.Itoa(done) + " 件完了"
	default:
		return "クリーンアップ完了: " + strconv.Itoa(done) + " 件 / " + atom.Bytes(bytes) + " を解放しました"
	}
}
