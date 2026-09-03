package cleanview_test

import (
	"context"
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/disk"
	"github.com/ousiassllc/gsr-helper/internal/exec"
	"github.com/ousiassllc/gsr-helper/internal/ui/molecule"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/disk/cleanview"
)

// docker の行名（cleanview.Rows）と進捗の表示名（disk.Apply の Progress.Label）が
// 一致していることを、**両辺で同じ定数を書かずに**確かめる（Issue #97）。
//
// 突き合わせ（cleanview.Mark）は表示名で行うため、internal/disk 側で進捗に載せる
// 名前だけが行名と分かれると、docker の行だけ永久に未着手で残り結果報告の件数も
// ずれる。両方が同じ定数を参照している限りコンパイルもテストも通ってしまうので、
// **実際に disk.Apply が届けた Progress をそのまま Mark へ流し、行の状態が進むか**
// で一致を見る。片方が別の文字列に分岐した時点でこのテストが落ちる。

// dockerOnlyPlan は docker だけの削除計画を返す。
func dockerOnlyPlan(t *testing.T) disk.CleanPlan {
	t.Helper()

	plan, err := disk.PlanClean([]disk.Target{{
		Label: disk.DockerLabel, Runner: "", Base: "", Path: "",
		Bytes: 4096, Files: -1, Docker: true, Protected: "",
	}})
	if err != nil {
		t.Fatalf("PlanClean がエラーを返した: %v", err)
	}
	if !plan.Docker || len(plan.Paths) != 0 {
		t.Fatalf("計画 = %+v, want docker のみ", plan)
	}
	return plan
}

// applyInto は計画を実行し、届いた進捗をそのまま行へ反映する。
func applyInto(t *testing.T, ex exec.Executor, plan disk.CleanPlan) []molecule.ProgressView {
	t.Helper()

	rows := cleanview.Rows(plan)
	if len(rows) != 1 {
		t.Fatalf("行数 = %d, want 1（docker のみ）", len(rows))
	}
	// 進捗の Label には触れない。ここで定数と比べてしまうと、両辺で同じ定数を
	// 使う既存テストと同じになり一致を検証しなくなる。
	_ = disk.Apply(context.Background(), ex, nil, plan, func(p disk.Progress) {
		cleanview.Mark(rows, p)
	})
	return rows
}

// 成功した docker の進捗が届けば行は完了になる。未着手のままなら名前が分かれている。
func TestApplyProgressMarksDockerRow(t *testing.T) {
	rows := applyInto(t, exec.NewFake(), dockerOnlyPlan(t))

	if rows[0].State != molecule.ProgressDone {
		t.Errorf("docker の行の状態 = %v, want 完了。"+
			"disk.Apply の Progress.Label と cleanview.Rows の行名が分かれている", rows[0].State)
	}
	done, failed, pending := cleanview.Counts(rows)
	if done != 1 || failed != 0 || pending != 0 {
		t.Errorf("件数 = 成功 %d / 失敗 %d / 未実行 %d, want 1 / 0 / 0", done, failed, pending)
	}
}

// 失敗した docker の進捗も同じ名前で届くので、行は失敗として残る。
func TestApplyProgressMarksDockerRowFailed(t *testing.T) {
	f := exec.NewFake()
	f.Push(exec.Result{Stdout: nil, Stderr: []byte("daemon 不応答"), ExitCode: 1}, nil)

	rows := applyInto(t, f, dockerOnlyPlan(t))

	if rows[0].State != molecule.ProgressFailed {
		t.Errorf("docker の行の状態 = %v, want 失敗。"+
			"disk.Apply の Progress.Label と cleanview.Rows の行名が分かれている", rows[0].State)
	}
	if rows[0].Detail == "" {
		t.Error("失敗の理由が行に出ていない")
	}
}
