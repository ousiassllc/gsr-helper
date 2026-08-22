package svc

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/ousiassllc/gsr-helper/internal/exec"
	"github.com/ousiassllc/gsr-helper/internal/runner"
	"github.com/ousiassllc/gsr-helper/internal/runner/procs"
)

// 待機は実時間にも実 /proc にも依らせない。Scan / Interval / Now を差し替え、
// 「何回目の走査で消えたか」だけで検証する。待ち時間が無制限（FR-07）である以上、
// 既定値のまま試すテストは終わらないかホストの状態に依存する。

// testInterval はポーリング間隔。実時間を待たない値にする。
const testInterval = time.Millisecond

// worker は r に属する Runner.Worker を作る。
func worker(r runner.Runner, pid int) procs.Process {
	return procs.Process{PID: pid, Kind: procs.Worker, Dir: r.Dir}
}

// scanner は与えた並びを 1 回の走査につき 1 つずつ返す関数を作る。
// 並びを使い切った後は最後の要素を返し続ける。
func scanner(rounds [][]procs.Process) (func() ([]procs.Process, error), *int) {
	calls := 0
	return func() ([]procs.Process, error) {
		i := min(calls, len(rounds)-1)
		calls++
		return rounds[i], nil
	}, &calls
}

// テスト用の Drainer を組む。
func drainer(f *exec.Fake, scan func() ([]procs.Process, error)) Drainer {
	return Drainer{Exec: f, Scan: scan, Interval: testInterval, Now: nil}
}

// Worker が消えるまで待ち、消えた時点で systemctl stop を発行する。
//
// 停止コマンドを打つのは 1 回だけで、待機中は 1 本も出さない。待機中に stop が出ると
// 「ジョブの完了を待ってから停止する」という操作の意味そのものが崩れる。
func TestDrainWaitsForWorkersThenStops(t *testing.T) {
	r := systemdRunner()
	f := exec.NewFake()
	scan, calls := scanner([][]procs.Process{
		{worker(r, 284193)},
		{worker(r, 284193)},
		{},
	})

	if err := drainer(f, scan).Drain(context.Background(), r, nil); err != nil {
		t.Fatalf("err = %v", err)
	}
	if *calls != 3 {
		t.Errorf("走査回数 = %d, want 3", *calls)
	}
	want := []string{"systemctl stop " + unitName}
	if got := cmdlines(f.Calls()); !slices.Equal(got, want) {
		t.Fatalf("発行コマンド\n got: %q\nwant: %q", got, want)
	}
	if o := f.Calls()[0].Options; o.Action != "svc.stop" || o.Runner != "build01-1" || o.SkipAudit {
		t.Errorf("監査メタ = %q/%q/%v, want svc.stop/build01-1/false", o.Action, o.Runner, o.SkipAudit)
	}
}

// 最初の走査で既に Worker が 0 件なら、間隔を待たずにそのまま停止へ進む。
func TestDrainStopsImmediatelyWhenIdle(t *testing.T) {
	r := systemdRunner()
	f := exec.NewFake()
	scan, calls := scanner([][]procs.Process{{}})

	// 間隔を長く取る。1 度でも待てばこのテストは終わらない。
	d := Drainer{Exec: f, Scan: scan, Interval: time.Hour, Now: nil}
	if err := d.Drain(context.Background(), r, nil); err != nil {
		t.Fatalf("err = %v", err)
	}
	if *calls != 1 {
		t.Errorf("走査回数 = %d, want 1", *calls)
	}
	if got := len(f.Calls()); got != 1 {
		t.Errorf("発行コマンド = %d 本, want 1 本", got)
	}
}

// 他の runner の Worker は待機の対象にしない。
//
// procs.Scan はホスト全体の runner プロセスを返すため、ディレクトリで絞らないと
// 隣の runner がジョブを抱えている間ずっと待ち続ける。Listener も対象外である
// （ドレイン停止が待つのはジョブであって受け口ではない）。
func TestDrainIgnoresOtherRunnersAndListener(t *testing.T) {
	r := systemdRunner()
	other := procs.Process{PID: 999, Kind: procs.Worker, Dir: "/opt/runners/build01-2"}
	listener := procs.Process{PID: 284102, Kind: procs.Listener, Dir: r.Dir}

	f := exec.NewFake()
	scan, calls := scanner([][]procs.Process{{other, listener}})

	if err := drainer(f, scan).Drain(context.Background(), r, nil); err != nil {
		t.Fatalf("err = %v", err)
	}
	if *calls != 1 {
		t.Errorf("走査回数 = %d, want 1（対象外のプロセスを待っている）", *calls)
	}
}

// 進捗は待機開始直後と各ポーリングごとに届き、残っている Worker と経過時間を持つ。
//
// 経過時間は Now を差し替えて固定する。実時計で見ると値を主張できない。
func TestDrainReportsProgress(t *testing.T) {
	r := systemdRunner()
	f := exec.NewFake()
	scan, _ := scanner([][]procs.Process{
		{worker(r, 284193), worker(r, 284194)},
		{worker(r, 284194)},
		{},
	})

	base := time.Date(2026, 8, 23, 10, 0, 0, 0, time.UTC)
	tick := 0
	now := func() time.Time {
		at := base.Add(time.Duration(tick) * time.Second)
		tick++
		return at
	}

	var got []Progress
	d := Drainer{Exec: f, Scan: scan, Interval: testInterval, Now: now}
	if err := d.Drain(context.Background(), r, func(p Progress) { got = append(got, p) }); err != nil {
		t.Fatalf("err = %v", err)
	}

	// 走査 3 回ぶん。Now は開始で 1 回、各進捗で 1 回呼ばれる。
	wantWorkers := []int{2, 1, 0}
	wantElapsed := []time.Duration{time.Second, 2 * time.Second, 3 * time.Second}
	if len(got) != len(wantWorkers) {
		t.Fatalf("進捗の件数 = %d, want %d", len(got), len(wantWorkers))
	}
	for i, p := range got {
		if len(p.Workers) != wantWorkers[i] {
			t.Errorf("%d 回目の Worker 件数 = %d, want %d", i, len(p.Workers), wantWorkers[i])
		}
		if p.Elapsed != wantElapsed[i] {
			t.Errorf("%d 回目の経過時間 = %v, want %v", i, p.Elapsed, wantElapsed[i])
		}
	}
}

// ctx のキャンセルで待機を中断し、**停止処理は行わない**。
//
// 待機を取り消したのであって停止を指示されたわけではないので、ここで stop を打つと
// 「キャンセルしたのにサービスが止まった」ことになる。
func TestDrainCancelStopsWithoutStopping(t *testing.T) {
	r := systemdRunner()
	f := exec.NewFake()
	ctx, cancel := context.WithCancel(context.Background())

	// 2 回目の走査を返した後にキャンセルする。Worker は消えないので、
	// キャンセルが無ければこの待機は終わらない。
	calls := 0
	scan := func() ([]procs.Process, error) {
		calls++
		if calls == 2 {
			cancel()
		}
		return []procs.Process{worker(r, 284193)}, nil
	}
	defer cancel()

	err := drainer(f, scan).Drain(ctx, r, nil)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
	if got := f.Calls(); len(got) != 0 {
		t.Errorf("キャンセル後に停止コマンドが発行されている: %q", cmdlines(got))
	}
}

// 走査に失敗したら待機を打ち切り、停止コマンドも発行しない。
//
// 「Worker が見えない」のと「見えていない」を取り違えると、ジョブの実行中に停止する。
func TestDrainAbortsOnScanError(t *testing.T) {
	f := exec.NewFake()
	want := errors.New("/proc を読めません")
	scan := func() ([]procs.Process, error) { return nil, want }

	err := drainer(f, scan).Drain(context.Background(), systemdRunner(), nil)
	if !errors.Is(err, want) {
		t.Errorf("err = %v, want %v を包んだもの", err, want)
	}
	if got := f.Calls(); len(got) != 0 {
		t.Errorf("走査に失敗したのに停止コマンドが発行されている: %q", cmdlines(got))
	}
}

// ユニット名が無ければ、待機に入らずに理由を返す。
//
// **走査が 1 度も呼ばれないことがこの検証の肝である。** 待ち切った先で発行するのは
// systemctl stop であり、ユニット名が分からなければ必ず失敗する。以前は run.sh 直起動の
// runner でも待機に入れたため、無制限（FR-07）に待った末に最初から分かっていた理由で
// 失敗を告げていた。
func TestDrainRefusesWithoutUnitBeforeWaiting(t *testing.T) {
	f := exec.NewFake()
	scans := 0
	scan := func() ([]procs.Process, error) {
		scans++
		return nil, nil
	}

	err := drainer(f, scan).Drain(context.Background(), standaloneRunner(), nil)
	if !errors.Is(err, ErrNoUnit) {
		t.Errorf("err = %v, want ErrNoUnit", err)
	}
	if scans != 0 {
		t.Errorf("走査回数 = %d, want 0（待ち切っても必ず失敗すると分かっているのに待ち始めている）", scans)
	}
	if got := f.Calls(); len(got) != 0 {
		t.Errorf("コマンドが発行されている: %q", cmdlines(got))
	}
}

// 既定の Drain は Drainer と同じ経路を通る（Executor と runner の受け渡しを取り違えない）。
//
// 走査は procs.Scan になるため、ホストの状態に依らないよう「即座に停止へ進む」経路
// だけを通す。実 /proc に Runner.Worker が居ればこのテストは待ち続けるが、テストを
// 走らせるホストで runner のジョブが動いていることは前提にしない。
func TestDrainUsesDefaults(t *testing.T) {
	f := exec.NewFake()
	r := systemdRunner()
	r.Dir = t.TempDir() // 実在するどの runner ディレクトリとも一致させない

	if err := Drain(context.Background(), f, r, nil); err != nil {
		t.Fatalf("err = %v", err)
	}
	want := []string{"systemctl stop " + unitName}
	if got := cmdlines(f.Calls()); !slices.Equal(got, want) {
		t.Errorf("発行コマンド\n got: %q\nwant: %q", got, want)
	}
}
