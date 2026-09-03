package svc

import (
	"context"
	"reflect"
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/exec"
	"github.com/ousiassllc/gsr-helper/internal/runner"
)

// enabledRunner は自動起動が有効な runner を返す。
func enabledRunner() runner.Runner {
	r := runningRunner()
	r.Svc = &runner.SvcState{Unit: unitName, Load: "loaded", Active: "active", Sub: "running", FileState: "enabled"}
	return r
}

// staticRunner は enable の対象にならないユニット（static）の runner を返す。
func staticRunner() runner.Runner {
	r := runningRunner()
	r.Svc = &runner.SvcState{Unit: unitName, Load: "loaded", Active: "active", Sub: "running", FileState: "static"}
	return r
}

// **確認ダイアログに出すコマンドと、実際に発行されるコマンドは一致していなければ
// ならない。** 食い違えば、利用者が承認した内容とは別のコマンドが走る。
//
// 表示（CommandLine）と実行（Start / Stop / Kill / Restart / Enable / Disable）を
// 同じ入力に対して突き合わせる。片方だけを直した退行はここで落ちる。
func TestCommandLineMatchesIssuedCommands(t *testing.T) {
	tests := map[string]struct {
		op   Op
		r    runner.Runner
		call func(context.Context, exec.Executor, runner.Runner) error
	}{
		"開始":           {OpStart, runningRunner(), Start},
		"停止":           {OpStop, runningRunner(), Stop},
		"再起動":          {OpRestart, runningRunner(), Restart},
		"強制停止":         {OpKill, runningRunner(), Kill},
		"enable への切替":  {OpEnable, staticRunner(), Enable},
		"disable への切替": {OpEnable, enabledRunner(), Disable},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			f := exec.NewFake()
			if err := tt.call(context.Background(), f, tt.r); err != nil {
				t.Fatalf("実行が失敗した: %v", err)
			}

			want := cmdlines(f.Calls())
			if got := CommandLine(tt.op, tt.r); !reflect.DeepEqual(got, want) {
				t.Errorf("CommandLine = %v, want %v（表示と実行が食い違っている）", got, want)
			}
		})
	}
}

// ユニット名が分からない runner には systemctl の行を出さない。
//
// 実行側も同じ条件で発行せず ErrNoUnit を返す（unitCommand）。出してしまうと、
// 「systemctl start 」という引数の欠けた行を承認させることになる。
func TestCommandLineOmitsUnitCommandsWithoutUnit(t *testing.T) {
	r := standaloneRunner()
	for _, op := range []Op{OpStart, OpStop, OpRestart, OpEnable, OpDrain} {
		if got := CommandLine(op, r); len(got) != 0 {
			t.Errorf("op %d のコマンド = %v, want 無し", op, got)
		}
	}
}

// 強制停止はユニットが無くてもプロセスへの kill を出す（Kill の doc）。
func TestCommandLineKillWorksWithoutUnit(t *testing.T) {
	r := standaloneRunner()
	r.Listener = &runner.Process{PID: 284102, Kind: runner.ProcListener, Dir: r.Dir}

	want := []string{"kill -KILL 284102"}
	if got := CommandLine(OpKill, r); !reflect.DeepEqual(got, want) {
		t.Errorf("CommandLine = %v, want %v", got, want)
	}
}

// 自動起動の判定は UnitFileState の enabled だけを有効とみなす。
func TestEnabled(t *testing.T) {
	tests := map[string]struct {
		r    runner.Runner
		want bool
	}{
		"enabled":   {enabledRunner(), true},
		"static":    {staticRunner(), false},
		"状態が取れていない": {runningRunner(), false},
		"disabled": {func() runner.Runner {
			r := runningRunner()
			r.Svc = &runner.SvcState{Unit: unitName, FileState: "disabled"}
			return r
		}(), false},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if got := Enabled(tt.r); got != tt.want {
				t.Errorf("Enabled = %v, want %v", got, tt.want)
			}
		})
	}
}
