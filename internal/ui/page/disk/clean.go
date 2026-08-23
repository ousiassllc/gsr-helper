package disk

import (
	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/disk"
	"github.com/ousiassllc/gsr-helper/internal/ui/organism/dialog"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/disk/cleanview"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/disk/confirmmodal"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/diskclean"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/progressmodal"
)

// クリーンアップ（FR-30）の一本道を実装する。
//
// **選択 → PlanClean（ドライラン）→ 確認 → Apply 以外の削除経路を作らない**
// （security.md「確認を経ない破壊的経路を作らない」）。この不変条件はコードの形で
// 守る。すなわち diskclean.Start を呼ぶのは startClean 1 か所だけで、startClean を
// 呼ぶのは dialog.DecidedMsg{Confirmed: true} を受けた onResult 1 か所だけである。
// c を押す requestClean は確認ダイアログを開く Cmd しか返さない。
//
// **実行そのものは page/diskclean が持つ**（Issue #102）。ここに残っているのは、
// 承認までの筋道（選択・ドライラン・確認・承認後の見直し）と、実行が終わったあとに
// タブの状態（選択・再集計・状態行）を戻す部分だけである。

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

// startClean は削除計画の実行を始め、進捗表示を開く。
func (m *Model) startClean(plan disk.CleanPlan) tea.Cmd {
	job, wait := diskclean.Start(m.tab, m.st.Exec, m.st.Audit, plan)
	m.clean = job
	m.notice = ""
	return tea.Batch(wait, m.openProgress())
}

// emptyPlan は「確認中の計画は無い」を表すゼロ値を返す。
func emptyPlan() disk.CleanPlan {
	return disk.CleanPlan{Paths: nil, Docker: false, Bytes: 0, Commands: nil}
}

// stopClean は実行中のクリーンアップを打ち切る。実行していなければ何もしない。
func (m *Model) stopClean() {
	if m.clean == nil {
		return
	}
	m.clean.Stop()
	m.clean = nil
}

// onProgress は進捗を ProgressList へ反映し、次の 1 件を待つ Cmd を返す。
//
// 全体件数は確認を通した計画の時点で確定しているため、進捗バーが出る
// （atomic-design.md の「bubbles/progress を使う範囲」）。
func (m *Model) onProgress(msg diskclean.ProgressMsg) tea.Cmd {
	if m.clean == nil {
		return nil
	}
	m.clean.Mark(msg)
	if !msg.OK {
		// channel が閉じた＝全対象を送り終えた。終了通知が既に届いていれば確定する。
		return m.finish()
	}
	return tea.Batch(m.clean.Next(m.tab), m.updateProgress())
}

// onApplyDone は終了通知を控え、確定を試みる（実際の確定は finish）。
func (m *Model) onApplyDone(msg diskclean.DoneMsg) tea.Cmd {
	if m.clean == nil {
		return nil
	}
	m.clean.Record(msg)
	return m.finish()
}

// finish は結果が確定した時点で報告を出し、選択を解いて再集計する。
//
// 再集計するのは使用量が変わったためである。残った表をそのまま出すと、消えた対象が
// 容量を持ったまま並び、もう一度選んで消せてしまうように見える。選択を解くのは、
// 消えた対象の選択が識別子ごと残って次のクリーンアップに紛れ込まないようにするため
// である。
//
// 裏へ回った後に終わった場合もここから張り直す。裏では畳むという原則からは外れるが、
// この集計は有限時間で必ず終わり、戻ったときに古い使用量を見せないほうが実害が
// 小さい（畳むために「今前面か」を持つと、状態が 1 つ増えて寿命の通知と二重管理になる）。
func (m *Model) finish() tea.Cmd {
	if m.clean == nil || !m.clean.Settled() {
		return nil
	}

	in, notice := m.clean.Settle()
	m.notice = notice
	report := progressmodal.Set(&m.overlay, in)
	stop := progressmodal.Stop(&m.overlay)

	m.clean = nil
	m.tbl.ClearSelection()
	return tea.Batch(report, stop, m.startScan())
}
