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

// **終了通知が最後の進捗より先に届いても件数がずれない。**
//
// 2 つは別の goroutine から届き、到着順が決まっていない。終了が届いた時点で数えると、
// まだ届いていない対象が「未実行」として報告に載り、進捗表示と状態行が違う件数を語る。
// 実際の配送順に依らず検証するため、**終了通知を先に投げてから**進捗を流す。
func TestFinalizeIsIndependentOfArrivalOrder(t *testing.T) {
	st, _ := baseState()
	rows := cleanview.Rows(plan2())

	m := newModel(t, st)
	m.clean = &cleanState{
		cancel: func() {}, ch: nil, done: 0, total: len(rows), bytes: 4096,
		rows: rows, report: nil, closed: false, result: nil,
	}

	// 終了通知が先。まだ 1 件も進捗が届いていないので確定してはならない。
	if cmd := m.onApplyDone(applyDoneMsg{err: nil}); cmd != nil {
		t.Fatal("進捗が 1 件も届く前に確定している")
	}

	// 進捗が後から全件届く。
	for i, r := range rows {
		m.onProgress(progressMsg{
			progress: disk.Progress{Label: r.Name, Done: i + 1, Total: len(rows)},
			ok:       true,
		})
	}
	// channel が閉じてはじめて確定する。
	if cmd := m.onProgress(progressMsg{ok: false}); cmd == nil {
		t.Fatal("進捗を出し切っても確定しない")
	}

	// 両者が同じ件数を語る（どちらも cleanview.Counts から数える）。
	if strings.Contains(m.notice, "未実行") {
		t.Errorf("状態行に未実行が残っている: %q", m.notice)
	}
	if !strings.Contains(m.notice, "2 件") {
		t.Errorf("状態行の件数 = %q, want 2 件（全件成功）", m.notice)
	}
}

// closed だけでも確定しない（result だけの逆方向は TestFinishWaitsForBothSignals）。
func TestFinishWaitsForResultToo(t *testing.T) {
	st, _ := baseState()
	m := newModel(t, st)
	m.clean = &cleanState{
		cancel: func() {}, ch: nil, done: 0, total: 1, bytes: 0,
		rows: cleanview.Rows(plan2()), report: nil, closed: true, result: nil,
	}

	if cmd := m.finish(); cmd != nil {
		t.Error("終了通知が来ていないのに確定している")
	}
	if m.clean == nil {
		t.Error("実行中の状態が捨てられている")
	}
}
