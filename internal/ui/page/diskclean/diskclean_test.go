package diskclean_test

import (
	"strings"
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/disk"
	"github.com/ousiassllc/gsr-helper/internal/exec"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/diskclean"
)

// 進捗の出し切りと終了通知がそろうまで結果を確定しないこと（Issue #75 / #102）を
// 検証する。2 つは別の goroutine から届き、**到着順が決まっていない。**

// dockerPlan は docker だけの計画を返す。
//
// ファイルを 1 つも触らないので、実行が実際に走っても消えるものが無い。削除が
// 起きたかどうかは exec.Fake の記録で確実に判定できる。
func dockerPlan() disk.CleanPlan {
	return disk.CleanPlan{
		Paths: nil, Docker: true, Bytes: 4096,
		Commands: [][]string{{"docker", "system", "prune", "-f"}},
	}
}

// 終了通知が最後の進捗より先に届いても件数がずれない。
//
// 終了が届いた時点で数えると、まだ届いていない対象が「未実行」として報告に載り、
// 進捗表示と状態行が違う件数を語る。**実際の配送順に依らず検証するため、終了通知を
// 先に投げてから**進捗を流す。
func TestJobSettlesOnlyAfterBothSignals(t *testing.T) {
	job, _ := diskclean.Start(0, exec.NewFake(), nil, dockerPlan())

	job.Record(diskclean.DoneMsg{Err: nil})
	if job.Settled() {
		t.Fatal("進捗が 1 件も届く前に確定している")
	}

	job.Mark(diskclean.ProgressMsg{
		Progress: disk.Progress{Label: disk.DockerLabel, Done: 1, Total: 1}, OK: true,
	})
	if job.Settled() {
		t.Fatal("channel が閉じる前に確定している（最後の進捗を取りこぼす）")
	}

	// channel が閉じてはじめて確定する。
	job.Mark(diskclean.ProgressMsg{Progress: disk.Progress{}, OK: false})
	if !job.Settled() {
		t.Fatal("両方そろっても確定しない")
	}

	in, notice, ok := job.Settle()
	if !ok {
		t.Fatal("両方そろっているのに Settle が確定しなかった")
	}
	if strings.Contains(notice, "未実行") {
		t.Errorf("状態行に未実行が残っている: %q", notice)
	}
	if !strings.Contains(notice, "1 件") {
		t.Errorf("状態行の件数 = %q, want 1 件（全件成功）", notice)
	}
	// 進捗表示と状態行はどちらも行の状態から数えるので、件数が食い違わない。
	if in.Done != 1 || in.Total != 1 {
		t.Errorf("進捗表示 = %d/%d, want 1/1", in.Done, in.Total)
	}
	if len(in.Report) == 0 {
		t.Error("結果報告が進捗表示に載っていない")
	}
}

// 進捗を出し切っても終了通知が無ければ確定しない（逆方向）。
func TestJobDoesNotSettleWithoutDone(t *testing.T) {
	job, _ := diskclean.Start(0, exec.NewFake(), nil, dockerPlan())

	job.Mark(diskclean.ProgressMsg{Progress: disk.Progress{}, OK: false})
	if job.Settled() {
		t.Error("終了通知が来ていないのに確定している")
	}
	job.Stop()
}

// 合図が片方しか届いていない Job は Settle が確定を拒む。
//
// **呼び出し側の作法ではなく Job 自身が弾くことを見る。** 判定を呼び出し側に委ねると、
// 終了通知だけが届いた時点で報告を組める形が残り、最後の対象が未着手のまま
// 「未実行 1 件」として数えられる（切り出す前は同じガードが確定処理と同じ関数の中に
// あった）。ここが緑である限り、呼び出し側の if を消しても不変条件は壊れない。
func TestSettleRefusesUnsettledJob(t *testing.T) {
	t.Run("終了通知だけ", func(t *testing.T) {
		job, _ := diskclean.Start(0, exec.NewFake(), nil, dockerPlan())
		job.Record(diskclean.DoneMsg{Err: nil})

		if _, _, ok := job.Settle(); ok {
			t.Error("進捗を出し切る前に確定した")
		}
		job.Stop()
	})

	t.Run("進捗の出し切りだけ", func(t *testing.T) {
		job, _ := diskclean.Start(0, exec.NewFake(), nil, dockerPlan())
		job.Mark(diskclean.ProgressMsg{Progress: disk.Progress{}, OK: false})

		if _, _, ok := job.Settle(); ok {
			t.Error("終了通知が来ていないのに確定した")
		}
		job.Stop()
	})
}

// 失敗した対象があれば解放量を出さず、失敗件数を書き分ける。
func TestJobNoticeWritesFailureInsteadOfFreedBytes(t *testing.T) {
	job, _ := diskclean.Start(0, exec.NewFake(), nil, dockerPlan())

	job.Mark(diskclean.ProgressMsg{
		Progress: disk.Progress{
			Label: disk.DockerLabel, Done: 1, Total: 1,
			Err: errFake("prune が終了コード 1 で終了しました"),
		},
		OK: true,
	})
	job.Mark(diskclean.ProgressMsg{Progress: disk.Progress{}, OK: false})
	job.Record(diskclean.DoneMsg{Err: errFake("prune が終了コード 1 で終了しました")})

	_, notice, ok := job.Settle()
	if !ok {
		t.Fatal("両方そろっているのに Settle が確定しなかった")
	}
	if !strings.Contains(notice, "1 件失敗") {
		t.Errorf("状態行 = %q, want 失敗件数を含む", notice)
	}
	if strings.Contains(notice, "解放しました") {
		t.Errorf("失敗があるのに解放量を出している: %q", notice)
	}
}

// errFake はテスト用のエラー。
type errFake string

func (e errFake) Error() string { return string(e) }
