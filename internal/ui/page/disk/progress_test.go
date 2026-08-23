package disk

import (
	"strings"
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/disk"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/disk/cleanview"
)

// plan2 は 2 件（パス 1 件 + docker）の計画を返す。
func plan2() disk.CleanPlan {
	return disk.CleanPlan{
		Paths: []disk.Target{{
			Label: "build01-1 / _work/foo", Runner: "build01-1",
			Base: "/opt/runners/build01-1", Path: "/opt/runners/build01-1/_work/foo",
			Bytes: 2048, Files: 3,
		}},
		Docker: true, Bytes: 4096, Commands: nil,
	}
}

// 実行中の件数は状態行に出さない（進捗表示と二重に出さない）。
func TestStatusLineDoesNotDuplicateProgress(t *testing.T) {
	m, _ := selected(t)
	m, _ = send(t, m, press("c"))
	m, _ = send(t, m, press("y"))

	if s := m.status(); strings.Contains(s, "クリーンアップ中") {
		t.Errorf("状態行に進捗が二重に出ている（status = %q）", s)
	}
}

// 終了通知が最後の進捗より先に届いても件数がずれない。
//
// 進捗を待つ Cmd と終了を待つ Cmd は別の goroutine から届き、到着順が決まっていない。
// 終了が届いた時点で数えると、まだ届いていない対象が「未実行」として報告に載り、
// 進捗表示と状態行が違う件数を出す。**両方そろうまで確定しない**ことで閉じている。
func TestResultIsNotFinalizedBeforeEveryProgressArrives(t *testing.T) {
	m, _ := selected(t)
	m, _ = send(t, m, press("c"))

	// y で実行が始まる。send は Cmd を流し切るので、この時点で確定まで進む。
	m, _ = send(t, m, press("y"))

	// 状態行と進捗表示の結果報告が同じ件数を語る（どちらも行の状態から数える）。
	status := m.status()
	if strings.Contains(status, "未実行") {
		t.Errorf("確定後に未実行が残っている（進捗を待たずに数えた）: %q", status)
	}
	if !strings.Contains(status, "クリーンアップ完了") {
		t.Errorf("結果が報告されていない: %q", status)
	}
	body := m.View().Content
	if strings.Contains(body, "未実行") {
		t.Errorf("結果報告に未実行が残っている:\n%s", body)
	}
}

// 確定後の状態行と結果報告が同じ件数を語る（どちらも行の状態から数える）。
func TestResultCountsComeFromOneSource(t *testing.T) {
	m, _ := selected(t)
	m, _ = send(t, m, press("c"))
	m, _ = send(t, m, press("y"))

	if s := m.status(); strings.Contains(s, "未実行") {
		t.Errorf("状態行に未実行が残っている（進捗を待たずに数えた）: %q", s)
	}
	if body := m.View().Content; strings.Contains(body, "未実行") {
		t.Errorf("結果報告に未実行が残っている:\n%s", body)
	}
}

// 片方の合図だけでは確定しない（進捗の出し切りと終了通知の両方を待つ）。
func TestFinishWaitsForBothSignals(t *testing.T) {
	st, _ := baseState()
	m := newModel(t, st)
	rows := cleanview.Rows(plan2())

	m.clean = &cleanState{
		cancel: func() {}, ch: nil, done: 0, total: len(rows), bytes: 4096,
		rows: rows, report: nil, closed: false, result: nil,
	}

	// 終了通知だけ: 最後の進捗をまだ受けていないので確定しない。
	m.clean.result = &applyDoneMsg{err: nil}
	if cmd := m.finish(); cmd != nil {
		t.Error("終了通知だけで確定している（最後の進捗を取りこぼす）")
	}
	if m.clean == nil {
		t.Fatal("実行中の状態が捨てられている")
	}

	// 進捗も出し切ったら確定する。
	m.clean.closed = true
	if cmd := m.finish(); cmd == nil {
		t.Error("両方そろっても確定しない")
	}
	if m.clean != nil {
		t.Error("確定後も実行中の状態が残っている")
	}
}
