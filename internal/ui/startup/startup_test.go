package startup_test

import (
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/appconfig"
	"github.com/ousiassllc/gsr-helper/internal/doctor"
	"github.com/ousiassllc/gsr-helper/internal/doctor/check"
	"github.com/ousiassllc/gsr-helper/internal/exec"
	"github.com/ousiassllc/gsr-helper/internal/runner"
	"github.com/ousiassllc/gsr-helper/internal/ui/hostreq"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/pagetest"
	"github.com/ousiassllc/gsr-helper/internal/ui/startup"
)

// 「いつ始めるか」だけを見る。何を確かめるか・何を集計するかはそれぞれの
// サブパッケージ（hostreq / workscan / ghscope）が検証する。

// newInput は 3 つとも発行できる入力を返す。
func newInput() startup.Input {
	return startup.Input{
		Runners: []runner.Runner{pagetest.SampleRunner()},
		Caps:    appconfig.Caps{GitHubToken: true},
		Exec:    exec.NewFake(),
	}
}

// newState は前提チェックを差し替えた State を返す（本物は実ホストを読む）。
func newState() startup.State {
	return startup.State{
		HostReq: hostreq.State{Checks: []doctor.Check{pagetest.StubCheck{Status: check.Fail}}},
	}
}

// 3 つとも発行でき、2 度目は 1 本も発行しない（FR-44 / Issue #73 / Issue #79）。
//
// **契機は「検出が成功した周期」であり初回とは限らない。** 呼び出し元
// （applyDiscovered）は成功のたびにここへ来るので、数え直さずに 2 度目が 0 本で
// 戻ることが「1 度きり」の担保である。3 秒ごとに走ると `sudo -l -U` が監査ログを
// 押し流し、_work の走査が再検出サイクルに載る。
func TestStartAllRunsEveryFetchOnce(t *testing.T) {
	t.Parallel()

	s := newState()
	if got := len(s.StartAll(newInput())); got != 3 {
		t.Fatalf("1 度目に発行した Cmd の本数 = %d, want 3（前提チェック・_work・保有スコープ）", got)
	}
	if got := len(s.StartAll(newInput())); got != 0 {
		t.Errorf("2 度目に発行した Cmd の本数 = %d, want 0（1 度きり）", got)
	}
}

// 発行できないものは束から落とす（nil を混ぜない）。
//
// **呼び出し側は本数で束ねるかを決める。** nil を混ぜて返すと、1 本も発行して
// いない周期でも tea.Batch が組まれ、共有状態の配布だけの Cmd の形が変わる
// （親の検証は 1 段展開で ChromeMsg を拾う。discover.go の applyDiscovered）。
func TestStartAllDropsWhatItCannotStart(t *testing.T) {
	t.Parallel()

	for name, tt := range map[string]struct {
		in   startup.Input
		want int
	}{
		"runner が 0 台なら _work の集計は出ない": {
			in:   startup.Input{Runners: nil, Caps: appconfig.Caps{GitHubToken: true}, Exec: exec.NewFake()},
			want: 2,
		},
		"トークンが無ければ保有スコープは出ない": {
			in: startup.Input{
				Runners: []runner.Runner{pagetest.SampleRunner()},
				Caps:    appconfig.Caps{GitHubToken: false},
				Exec:    exec.NewFake(),
			},
			want: 2,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			s := newState()
			cmds := s.StartAll(tt.in)
			if len(cmds) != tt.want {
				t.Fatalf("発行した Cmd の本数 = %d, want %d", len(cmds), tt.want)
			}
			for i, c := range cmds {
				if c == nil {
					t.Errorf("%d 本目が nil（発行できなかったものは落とす）", i)
				}
			}
		})
	}
}
