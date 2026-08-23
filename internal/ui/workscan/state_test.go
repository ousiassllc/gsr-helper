package workscan_test

import (
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/runner"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
	"github.com/ousiassllc/gsr-helper/internal/ui/workscan"
)

// 集計の進行状況（周期の突き合わせと二重起動の抑止）を検証する。

func TestStateDoesNotOverlapRuns(t *testing.T) {
	var s workscan.State
	rs := []runner.Runner{runnerAt(t.TempDir())}

	if s.Start(rs) == nil {
		t.Fatal("1 本目が始まらない")
	}
	if s.Start(rs) != nil {
		t.Error("実行中なのに 2 本目が始まっている（連打で goroutine が積み上がる）")
	}
}

func TestStateDropsStaleCycle(t *testing.T) {
	var s workscan.State
	rs := []runner.Runner{runnerAt(t.TempDir())}
	s.Start(rs)

	// 追い抜かれた周期。取り込むと消えた対象の使用量が新しい一覧へ混ざる。
	s.Apply(workscan.Msg{Seq: 0, Usage: map[string]page.WorkUsage{"/old": {Bytes: 1}}})
	if len(s.Usage()) != 0 {
		t.Errorf("古い周期の結果を取り込んでいる: %v", s.Usage())
	}

	s.Apply(workscan.Msg{Seq: 1, Usage: map[string]page.WorkUsage{"/new": {Bytes: 2}}})
	if got := s.Usage()["/new"].Bytes; got != 2 {
		t.Errorf("現在の周期の結果 = %d, want 2", got)
	}
	// 取り込んだら次を始められる。
	if s.Start(rs) == nil {
		t.Error("完了後に再集計を始められない")
	}
}

// runner が 0 台なら周期を進めず、次の契機へ譲る。
func TestStateWithoutRunnersDoesNotAdvance(t *testing.T) {
	var s workscan.State

	if s.Start(nil) != nil {
		t.Error("runner 0 台で Cmd を発行している")
	}
	if s.Started() {
		t.Error("周期が進んでいる（次の契機で始められなくなる）")
	}
}
