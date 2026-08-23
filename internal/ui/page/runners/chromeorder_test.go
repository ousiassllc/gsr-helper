package runners_test

import (
	"strings"
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/ui/organism/dialog"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/action"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/runnerop"
)

// **1 回の Update が返す ChromeMsg そのもの**を見る検証を集める。
//
// ops_test.go の opsChrome は「状態を変えない Msg を 1 つ送って chrome を取り直す」
// 形なので、ChromeMsg が 1 往復遅れて正しくなる退行を検知できない。ここでは打鍵や
// 決定を処理したその Update の戻り値から ChromeMsg を取り出し、**その場で**正しい
// 内容が載っていることを固定する。
//
// 固定したいのは tea.Batch の引数の評価順である。Go は関数呼び出しの引数を左から
// 右へ評価するので、tea.Batch(m.chrome(), m.ops.Update(msg)) と書くと chrome は
// ops が変わる**前**の状態を読む。状態行は既定 3 秒後の page.StateMsg まで空のまま、
// 閉じたはずの確認ダイアログは Modal=true のまま残る。

// 操作の完了（DoneMsg）を処理した Update が返す ChromeMsg に、その結果が載る。
//
// handleOps は先に選択を解くため、結果が載らないと状態行は「選択: N 件」も消えた
// 空白になる（一括停止の成否を読む手がかりが 3 秒間どこにも無い）。
func TestDoneMsgChromeCarriesResultImmediately(t *testing.T) {
	st, _ := opsState()
	m := newOpsModel(t, st)

	done := runnerop.Msg{Payload: runnerop.DoneMsg{
		Op:      action.Stop,
		Results: []runnerop.Result{{Runner: "build01-1", Err: nil}},
	}}
	_, cmd := m.Update(done)

	got := chrome(t, cmd).Status
	if got == "" {
		t.Fatal("DoneMsg を処理した Update の ChromeMsg に結果が載っていない（状態行が空のまま）")
	}
	if !strings.Contains(got, "停止") {
		t.Errorf("ChromeMsg.Status = %q, 操作名（停止）を含むべき", got)
	}
}

// 確認のキャンセルを処理した Update が返す ChromeMsg で、モーダルが閉じている。
//
// runnerop.Model.Result はキャンセルでも overlay.Close() を呼ぶ。それより先に
// chrome を組むと Modal=true のままになり、ダイアログのフッタヒントが最大 3 秒
// 残ったままになる。
func TestConfirmCancelChromeClosesModalImmediately(t *testing.T) {
	st, _ := opsState()
	m := newOpsModel(t, st)

	m = opsSend(t, m, "x")
	if !opsChrome(t, m).Modal {
		t.Fatal("確認ダイアログが開いていない（前提が崩れている）")
	}

	_, cmd := m.Update(page.ResultMsg{Kind: runnerop.ConfirmKind, Msg: dialog.DecidedMsg{Confirmed: false}})
	if chrome(t, cmd).Modal {
		t.Error("キャンセルを処理した Update の ChromeMsg が Modal=true のまま")
	}
}
