package disk

import (
	"context"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/disk"
	"github.com/ousiassllc/gsr-helper/internal/ui/atom"
	"github.com/ousiassllc/gsr-helper/internal/ui/organism/dialog"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
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
	// workDirName は削除の可否がジョブの有無で変わるサブツリー。
	//
	// internal/disk と同じ名前を持つのは、保護の判定をどちらの層でも同じ
	// サブツリーに対して行うためである。
	workDirName = "_work"
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
	// done / total は状態行に出す進捗（"クリーンアップ中 (2/5)"）。
	done  int
	total int
	// bytes は解放見込み。結果報告に使う。
	bytes int64
}

// progressMsg は進捗 1 件の到着。ok が偽なら channel が閉じたことを表す。
type progressMsg struct {
	progress disk.Progress
	ok       bool
}

// applyDoneMsg はクリーンアップの終了。
//
// 失敗件数を進捗の受信側で数えずに実行側から受け取るのは、進捗を待つ Cmd と
// 終了を待つ Cmd の到着順が決まっていないためである。受信側で数えると、最後の
// 進捗より先に終了が届いた回だけ報告の件数が 1 件ずれる。
type applyDoneMsg struct {
	err    error
	failed int
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

	plan, err := disk.PlanClean(cleanTargets(checked))
	if err != nil {
		m.notice = err.Error()
		return nil
	}

	m.plan = plan
	return openConfirm(&m.overlay, confirmInput(plan))
}

// onResult は確認ダイアログの決定を処理する。
//
// **実行へ進む唯一の分岐である。** Confirmed が真のときだけ startClean を呼ぶ。
// 偽（n / esc / enter）のときは計画を捨てて閉じるだけで、Executor もファイルシステムも
// 触らない。
func (m *Model) onResult(msg page.ResultMsg) tea.Cmd {
	decided, ok := msg.Msg.(dialog.DecidedMsg)
	if msg.Kind != confirmKind || !ok {
		return nil
	}

	m.overlay.Close()
	plan := m.plan
	m.plan = emptyPlan()
	if !decided.Confirmed {
		return nil
	}
	// 承認を待つ間にジョブが始まっていないかを見直す（reprotected の doc）。
	if label := reprotected(plan, m.st.Result.Runners); label != "" {
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
		failed := 0
		err := disk.Apply(ctx, ex, lg, plan, func(p disk.Progress) {
			if p.Err != nil {
				failed++
			}
			ch <- p
		})
		close(ch)
		doneCh <- applyDoneMsg{err: err, failed: failed}
	}()

	m.clean = &cleanState{cancel: cancel, ch: ch, done: 0, total: total, bytes: plan.Bytes}
	m.notice = ""
	return tea.Batch(m.waitProgress(ch), m.waitApply(doneCh))
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

// onProgress は進捗を状態行へ反映し、次の 1 件を待つ Cmd を返す。
//
// 進捗バーは出さない。全体件数は確定しているので出せる形ではあるが（screens.md の
// 「全体件数が確定している処理では進捗バーを併記する」）、バーを描く部品
// （ProgressList）がまだ無い。件数だけを出しておき、部品ができた時点で差し替える。
func (m *Model) onProgress(msg progressMsg) tea.Cmd {
	if m.clean == nil || !msg.ok {
		return nil
	}
	m.clean.done = msg.progress.Done
	return m.waitProgress(m.clean.ch)
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

	m.notice = cleanNotice(m.clean.total, m.clean.bytes, msg)
	m.clean.cancel()
	m.clean = nil
	m.tbl.ClearSelection()
	return m.startScan()
}

// cleanNotice は結果報告の 1 行を返す。
//
// 失敗があるときに解放量を出さないのは、消せなかった対象のぶんが含まれた見込み値に
// なるためである。見込みと実測が食い違う数字を「解放しました」と書くと、次に何をす
// べきかの判断を誤らせる。
func cleanNotice(total int, bytes int64, msg applyDoneMsg) string {
	if msg.failed > 0 {
		return "クリーンアップ完了: " + strconv.Itoa(total-msg.failed) + " 件成功 / " +
			strconv.Itoa(msg.failed) + " 件失敗"
	}
	if msg.err != nil {
		return "クリーンアップに失敗しました: " + firstLine(msg.err.Error())
	}
	return "クリーンアップ完了: " + strconv.Itoa(total) + " 件 / " + atom.Bytes(bytes) + " を解放しました"
}

// firstLine は 1 行目だけを返す。続きがあることは中略記号で示す。
//
// disk.Apply は errors.Join で失敗を束ねる（改行区切り）ため、そのまま状態行へ流すと
// 1 行の領域に複数行が入って枠が崩れる。
func firstLine(s string) string {
	head, rest, found := strings.Cut(s, "\n")
	if found && rest != "" {
		return head + " …"
	}
	return head
}
