package hostcaps

import (
	"context"
	"testing"
	"time"

	"github.com/ousiassllc/gsr-helper/internal/exec"
)

// 起動時の能力判定にかかる時間の性質——1 本ごとの期限・全体の打ち切り・並行実行——を
// 検証する。
//
// 判定結果そのもの（hostcaps_test.go）と分けているのは、**壊れたときの現れ方が
// 違う**ためである。あちらが壊れると Caps の中身が誤り、こちらが壊れると起動が
// 遅くなる（あるいは応答しないホストで固まる）。1 ファイル 300 行の上限
// （docs/ui/atomic-design.md）に収める際の切れ目もここになる（Issue #114）。

// Detect が Options.Timeout をコマンドの期限に反映し、全体では detectBudget で
// 打ち切ること。Fake は Call に deadline を記録するので、これだけで検証できる。
func TestDetectAppliesTimeout(t *testing.T) {
	for _, tt := range []struct {
		name    string
		timeout time.Duration
		want    time.Duration
	}{
		// 未指定時の既定は 500 ms（defaultProbeTimeout）。仕様書
		// （docs/components/overview.md / non-functional.md）が定める値であり、
		// コメントだけがこの値から離れていた経緯があるため値で固定する。
		{name: "未指定なら既定の 500 ms", timeout: 0, want: 500 * time.Millisecond},
		{name: "Options.Timeout が 1 コマンドの上限になる", timeout: 300 * time.Millisecond, want: 300 * time.Millisecond},
		{name: "長すぎる指定は全体予算で打ち切る", timeout: time.Hour, want: detectBudget},
	} {
		t.Run(tt.name, func(t *testing.T) {
			fakePATH(t)
			f := exec.NewFake()
			f.SetFunc(func(_ string, _ []string) (exec.Result, error) { return okResult() })

			Detect(context.Background(), f, Options{HasToken: nil, Timeout: tt.timeout})
			calls := f.Calls()
			if len(calls) == 0 {
				t.Fatal("コマンドが発行されていない")
			}
			for _, c := range calls {
				if !c.HasDeadline {
					t.Fatalf("期限のない ctx で実行している: %v", c)
				}
				if remaining := time.Until(c.Deadline); remaining <= 0 || remaining > tt.want {
					t.Errorf("%v の残り時間 = %v, want 0 < x <= %v", c, remaining, tt.want)
				}
			}
		})
	}
}

// docker とトークンの判定は並行実行すること。逐次だと最悪 3 本分待つことになり
// 起動時間の目標に収まらない。実時間ではなく「2 本が同時に走っていること」で見る。
func TestDetectRunsProbesConcurrently(t *testing.T) {
	f := exec.NewFake()
	arrived := make(chan struct{}, 2)
	release := make(chan struct{})
	f.SetFunc(func(_ string, _ []string) (exec.Result, error) {
		arrived <- struct{}{}
		<-release // 2 本目が来るまで返さない。逐次実装ならここで詰まる
		return okResult()
	})

	// トークン判定は注入されたものが発行する。docker と合わせて 2 本になる。
	p := testProbes([]string{"docker", "gh"}, nil, 1000)
	p.hasToken = func(ctx context.Context, ex exec.Executor, _ time.Duration) bool {
		res, err := ex.Run(ctx, "gh", "auth", "token")
		return err == nil && res.ExitCode == 0
	}

	done := make(chan Caps, 1)
	go func() {
		done <- detect(context.Background(), f, p, testTimeout)
	}()

	for i := 1; i <= 2; i++ {
		select {
		case <-arrived:
		case <-time.After(testTimeout):
			close(release)
			t.Fatalf("%d 本目のコマンドが発行されない（逐次実行になっている）", i)
		}
	}
	close(release)

	if got := <-done; !got.Docker || !got.GitHubToken {
		t.Errorf("Docker/GitHubToken = %v/%v, want true/true", got.Docker, got.GitHubToken)
	}
}

// Timeout 未指定なら既定値が使われること。
func TestProbeTimeout(t *testing.T) {
	for _, tt := range []struct {
		name string
		in   time.Duration
		want time.Duration
	}{
		{name: "未指定", in: 0, want: defaultProbeTimeout},
		{name: "負値", in: -time.Second, want: defaultProbeTimeout},
		{name: "指定あり", in: testTimeout, want: testTimeout},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := probeTimeout(tt.in); got != tt.want {
				t.Errorf("probeTimeout(%v) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}
