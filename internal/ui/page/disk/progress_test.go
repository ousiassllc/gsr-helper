package disk

import (
	"strings"
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/disk"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/diskclean"
)

// dockerPlan は docker だけの計画を返す（ファイルを 1 つも触らない）。
func dockerPlan() disk.CleanPlan {
	return disk.CleanPlan{
		Paths: nil, Docker: true, Bytes: 4096,
		Commands: [][]string{{"docker", "system", "prune", "-f"}},
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

// 両方の合図がそろうまでタブも確定しない。そろったら実行中の状態を捨てて報告する。
//
// 確定の条件そのものは page/diskclean の Job が持つ（そちらのテストが到着順に依らない
// ことを見る）。ここで見るのは**タブ側の後始末**——報告を状態行へ出し、選択を解き、
// 実行中の状態を捨てるところである。
func TestFinishClearsRunningStateOnBothSignals(t *testing.T) {
	st, _ := dockerState()
	m := newModel(t, st)

	m.startClean(dockerPlan())
	if m.clean == nil {
		t.Fatal("実行中の状態が無い（前提が崩れている）")
	}

	// 終了通知が先に届いても確定しない（最後の進捗を取りこぼす）。
	if cmd := m.onApplyDone(diskclean.DoneMsg{Err: nil}); cmd != nil {
		t.Fatal("進捗を出し切る前に確定している")
	}
	if m.clean == nil {
		t.Fatal("実行中の状態が捨てられている")
	}

	m.onProgress(diskclean.ProgressMsg{
		Progress: disk.Progress{Label: disk.DockerLabel, Done: 1, Total: 1}, OK: true,
	})
	if cmd := m.onProgress(diskclean.ProgressMsg{Progress: disk.Progress{}, OK: false}); cmd == nil {
		t.Fatal("両方そろっても確定しない")
	}

	if m.clean != nil {
		t.Error("確定後も実行中の状態が残っている")
	}
	if !strings.Contains(m.notice, "クリーンアップ完了") {
		t.Errorf("結果が報告されていない（notice = %q）", m.notice)
	}
	if strings.Contains(m.notice, "未実行") {
		t.Errorf("状態行に未実行が残っている: %q", m.notice)
	}
}
