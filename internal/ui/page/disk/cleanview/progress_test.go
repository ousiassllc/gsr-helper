package cleanview_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/disk"
	"github.com/ousiassllc/gsr-helper/internal/ui/molecule"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/disk/cleanview"
)

// クリーンアップ進捗の ProgressList 表示（Issue #75）を検証する。

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

// 分母は計画の件数になる（件数が確定しているのでバーが出る）。
func TestCleanRowsCoverEveryTargetInPlanOrder(t *testing.T) {
	rows := cleanview.Rows(plan2())

	if len(rows) != 2 {
		t.Fatalf("行数 = %d, want 2（パス 1 件 + docker）", len(rows))
	}
	// 並びは disk.Apply が処理する順（パス → docker）。
	if rows[0].Name != "build01-1 / _work/foo" {
		t.Errorf("1 行目 = %q, want パスの対象", rows[0].Name)
	}
	if rows[1].Name != cleanview.DockerLabel {
		t.Errorf("2 行目 = %q, want %q", rows[1].Name, cleanview.DockerLabel)
	}
	for i, r := range rows {
		if r.State != molecule.ProgressWaiting {
			t.Errorf("%d 行目の初期状態 = %v, want 未着手", i, r.State)
		}
	}
}

// 進捗は名前で突き合わせて行の状態を進める。失敗した対象は失敗として残る。
func TestMarkRowReflectsOutcomePerTarget(t *testing.T) {
	rows := cleanview.Rows(plan2())

	cleanview.Mark(rows, disk.Progress{Label: "build01-1 / _work/foo", Done: 1, Total: 2})
	cleanview.Mark(rows, disk.Progress{Label: cleanview.DockerLabel, Done: 2, Total: 2,
		Err: errors.New("prune が終了コード 1 で終了しました")})

	if rows[0].State != molecule.ProgressDone {
		t.Errorf("成功した対象の状態 = %v, want 完了", rows[0].State)
	}
	if rows[1].State != molecule.ProgressFailed {
		t.Errorf("失敗した対象の状態 = %v, want 失敗", rows[1].State)
	}
	if !strings.Contains(rows[1].Detail, "終了コード 1") {
		t.Errorf("失敗の理由が行に出ていない（Detail = %q）", rows[1].Detail)
	}
	// 知らない名前は黙って捨てる（削除そのものは進んでいる）。
	cleanview.Mark(rows, disk.Progress{Label: "知らない対象", Done: 3, Total: 2})
}

// 完了後の結果報告に成功・失敗・未実行が出る。
func TestCleanReportWritesEveryOutcome(t *testing.T) {
	tests := []struct {
		name   string
		states []molecule.ProgressState
		err    error
		want   []string
		absent []string
	}{
		{
			name:   "全件成功なら解放量まで出す",
			states: []molecule.ProgressState{molecule.ProgressDone, molecule.ProgressDone},
			want:   []string{"成功: 2 件", "解放: "},
			absent: []string{"失敗", "未実行"},
		},
		{
			// 失敗があると解放量は出さない（消せなかった対象を含む見込み値になる）。
			name:   "失敗があれば件数を書き分け解放量は出さない",
			states: []molecule.ProgressState{molecule.ProgressDone, molecule.ProgressFailed},
			want:   []string{"成功: 1 件", "失敗: 1 件"},
			absent: []string{"解放: "},
		},
		{
			// 打ち切られると残りが未着手のまま残る。
			name:   "打ち切られたら未実行を出す",
			states: []molecule.ProgressState{molecule.ProgressDone, molecule.ProgressWaiting},
			err:    errors.New("クリーンアップを中断しました"),
			want:   []string{"成功: 1 件", "未実行: 1 件"},
			absent: []string{"解放: "},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rows := cleanview.Rows(plan2())
			for i := range rows {
				rows[i].State = tt.states[i]
			}

			got := strings.Join(cleanview.Report(rows, 4096, tt.err), "\n")
			for _, want := range tt.want {
				if !strings.Contains(got, want) {
					t.Errorf("結果報告に %q が無い:\n%s", want, got)
				}
			}
			for _, absent := range tt.absent {
				if strings.Contains(got, absent) {
					t.Errorf("結果報告に %q が出ている:\n%s", absent, got)
				}
			}
		})
	}
}
