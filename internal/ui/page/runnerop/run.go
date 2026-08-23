package runnerop

import (
	"context"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/exec"
	"github.com/ousiassllc/gsr-helper/internal/runner"
	"github.com/ousiassllc/gsr-helper/internal/svc"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/action"
)

// 操作の実行と結果の報告を集める。

// Msg は runnerop が自分宛に発行した Msg の包み。
//
// **タブは case runnerop.Msg で受け、Model.Update へ渡すこと。** 包まないと、
// モーダルを 1 枚でも開いている間は page.Overlay.Handles が既定で真を返す
// （page/overlay.go）ため、待機中の Msg が待機画面に吸われて制御部へ届かない。
// ドレイン停止はまさに待機画面を開いたまま結果を待つので、包まない実装は
// 「待機が永久に終わらない」形で壊れる。
type Msg struct {
	// Payload は中身。一括操作の完了は DoneMsg で、それ以外は内部の段取りである。
	Payload tea.Msg
}

// Result は 1 件の実行結果。
type Result struct {
	Runner string // 対象 runner 名
	Err    error  // 失敗の理由。成功なら nil
}

// DoneMsg は一括操作の完了。Msg.Payload として page へ届く。
//
// 全件ぶんの結果を載せる。**1 件失敗しても残りを実行する**（run の doc）ので、
// 成功と失敗が混ざった結果をそのまま報告できる形にしてある。
type DoneMsg struct {
	Op      action.ID
	Results []Result
}

// note は次の DoneMsg に添える補足。
//
// 結果そのものではないため Result にも DoneMsg にも載せない。可否の再判定で
// 外した件数は実行の**前**に、待機のキャンセルは実行の**途中**に決まり、
// どちらも「何件成功したか」とは別の情報である。
type note struct {
	skipped  int  // 可否の再判定で対象から外した件数
	canceled bool // ドレイン待機をキャンセルしたか
}

// run は対象へ順に操作を適用する Cmd を返す。
//
// **1 件失敗しても残りを続ける。** メンテナンス前の全停止（FR-08）で 1 台の失敗が
// 残りを止めると、止まった台と止まらなかった台が混在したまま作業に入ることになり、
// 運用が破綻する。全件の結果を DoneMsg に載せ、どれが失敗したかを報告する。
//
// 対象の並びは呼び出し側が決めた順（一覧の並び）のままにする。並べ替えると、
// 確認ダイアログに出したコマンドの順と実行の順が食い違う。
func (m *Model) run(op action.ID, targets []runner.Runner) tea.Cmd {
	sop, ok := action.SvcOp(op)
	if !ok {
		return nil
	}

	ex := m.st.Exec
	return page.Do(m.tab, func() tea.Msg {
		results := make([]Result, 0, len(targets))
		for _, r := range targets {
			results = append(results, Result{Runner: r.Name(), Err: apply(context.Background(), ex, sop, r)})
		}
		return Msg{Payload: DoneMsg{Op: op, Results: results}}
	})
}

// apply は 1 件へ操作を適用する。
//
// 分岐を svc.Op で行うのは、確認ダイアログのコマンド表示（svc.CommandLine）と
// 同じ識別子を使うためである。action.ID のまま分岐すると、表示と実行で別々の
// 対応表を持つことになり、片方だけが新しい操作を知っている状態を作れてしまう。
//
// **enable / disable は 1 つのキー（E）で切り替える。** 今の状態が enabled なら
// disable、それ以外（disabled / static / 状態不明）なら enable である。判定の根拠は
// svc.Enabled が持ち、コマンドの表示（svc.CommandLine の OpEnable）と共有する。
//
// ctx に締切を設けないのは、締切を課すのが Executor 側の責務だからである
// （internal/exec/command。1 回の実行ごとに予算を持つ）。ここで重ねると、
// 一括操作の件数に依らない一定の予算が全体にかかる。
func apply(ctx context.Context, ex exec.Executor, op svc.Op, r runner.Runner) error {
	switch op {
	case svc.OpStart:
		return svc.Start(ctx, ex, r)
	case svc.OpStop:
		return svc.Stop(ctx, ex, r)
	case svc.OpKill:
		return svc.Kill(ctx, ex, r)
	case svc.OpRestart:
		return svc.Restart(ctx, ex, r)
	case svc.OpEnable:
		if svc.Enabled(r) {
			return svc.Disable(ctx, ex, r)
		}
		return svc.Enable(ctx, ex, r)
	case svc.OpDrain:
		// ドレイン停止は待機を伴うので drain.go が svc.Drain を直に呼ぶ。
		return nil
	default:
		return nil
	}
}

// describe は一括操作の結果を状態行の 1 行にする。
//
// 成功件数だけを出さない。失敗した runner の名前と理由まで出さないと、利用者は
// 「何台か落ちなかった」ことは分かっても、どれを手で追えばよいか分からない
// （functional.md の確認フローの「結果報告」）。
func describe(name string, results []Result, n note) string {
	ok := 0
	failed := make([]string, 0, len(results))
	for _, r := range results {
		if r.Err == nil {
			ok++
			continue
		}
		failed = append(failed, r.Runner+": "+firstLine(r.Err.Error()))
	}

	parts := make([]string, 0, 4)
	if n.canceled {
		parts = append(parts, "キャンセルしました")
	}
	// キャンセルして 1 件も終わっていないときに「0 件成功」を出さない。何も
	// 起きなかったことは「キャンセルしました」だけで足りる。
	if ok > 0 || (len(failed) == 0 && !n.canceled) {
		parts = append(parts, strconv.Itoa(ok)+" 件成功")
	}
	if len(failed) > 0 {
		parts = append(parts, strconv.Itoa(len(failed))+" 件失敗（"+strings.Join(failed, " / ")+"）")
	}
	if n.skipped > 0 {
		parts = append(parts, strconv.Itoa(n.skipped)+" 件は実行できないため除外")
	}
	return name + ": " + strings.Join(parts, " ")
}

// noTargetText は実行できる対象が 1 件も残らなかったときの文を返す。
//
// 理由まで出すのは、押しても何も起きないように見えることを避けるためである
// （screens.md の設計原則 2）。理由の文言は svc.CanControl / action.Allow が持つ
// ものをそのまま使い、ここでは言い換えない。
func noTargetText(total int, reason string) string {
	if total == 0 {
		return "対象がありません"
	}
	if reason == "" {
		reason = page.ReasonUnsupported
	}
	return "実行できる対象がありません（" + reason + "）"
}

// firstLine は複数行のエラー文を 1 行目だけにする。
//
// 強制停止は kill と systemctl stop の失敗を errors.Join でまとめる（svc.Kill）ため、
// 改行を含む文字列になりうる。状態行は 1 行しか無く、改行をそのまま出すと枠が崩れる。
func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i] + " …"
	}
	return s
}
