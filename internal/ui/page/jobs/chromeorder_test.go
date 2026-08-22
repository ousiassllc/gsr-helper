package jobs_test

import (
	"strings"
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/ui/organism/dialog"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/action"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/runnerop"
)

// **1 回の Update が返す ChromeMsg そのもの**を見る検証を集める
// （runners/chromeorder_test.go と同じ理由・同じ観点）。
//
// ops_test.go の opsChrome は状態を変えない Msg を 1 つ挟んでから chrome を取る形で、
// ChromeMsg が 1 往復遅れて正しくなる退行を検知できない。Jobs タブも Runners タブと
// 同じ経路（page/runnerop）を通す以上、同じ退行が同じ形で起きうるため、片方だけ
// 固定しない。

// 操作の完了（DoneMsg）を処理した Update が返す ChromeMsg に、その結果が載る。
func TestJobsDoneMsgChromeCarriesResultImmediately(t *testing.T) {
	m, _ := opsModel(t, busyRunner("build01-1", 1))

	done := runnerop.Msg{Payload: runnerop.DoneMsg{
		Op:      action.Restart,
		Results: []runnerop.Result{{Runner: "build01-1", Err: nil}},
	}}
	_, cmd := m.Update(done)

	got := chrome(t, cmd).Status
	if got == "" {
		t.Fatal("DoneMsg を処理した Update の ChromeMsg に結果が載っていない（状態行が空のまま）")
	}
	if !strings.Contains(got, "再起動") {
		t.Errorf("ChromeMsg.Status = %q, 操作名（再起動）を含むべき", got)
	}
}

// 確認のキャンセルを処理した Update が返す ChromeMsg で、モーダルが閉じている。
func TestJobsConfirmCancelChromeClosesModalImmediately(t *testing.T) {
	m, _ := opsModel(t, busyRunner("build01-1", 1))

	m = opsSend(t, m, "R")
	if !opsChrome(t, m).Modal {
		t.Fatal("確認ダイアログが開いていない（前提が崩れている）")
	}

	_, cmd := m.Update(page.ResultMsg{Kind: runnerop.ConfirmKind, Msg: dialog.ConfirmedMsg{OK: false}})
	if chrome(t, cmd).Modal {
		t.Error("キャンセルを処理した Update の ChromeMsg が Modal=true のまま")
	}
}
