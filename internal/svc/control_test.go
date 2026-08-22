package svc

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/exec"
	"github.com/ousiassllc/gsr-helper/internal/runner"
)

// 発行コマンドの検証は exec.Fake の記録（Calls）で行う。
//
// **監査ログのメタ情報も Calls から見る。** Fake.SetFunc のコールバックは ctx を
// 受け取らない設計なので（exec.Fake の doc）、その中で exec.OptionsFrom を呼ぶことは
// できない。Fake.Run は記録の時点で exec.OptionsFrom(ctx) の結果を Call.Options に
// 写しており、見ている値は同じものである。

// unitName は検証に使う systemd ユニット名。
const unitName = "actions.runner.foo.build01-1.service"

// runningRunner は Listener と Worker が 1 つずつ動いている runner を返す。
func runningRunner() runner.Runner {
	r := systemdRunner()
	r.Listener = &runner.Process{PID: 284102, Kind: runner.ProcListener, Dir: r.Dir}
	r.Workers = []runner.Process{{PID: 284193, Kind: runner.ProcWorker, Dir: r.Dir}}
	return r
}

// cmdlines は記録された呼び出しをコマンド行の並びで返す。
func cmdlines(calls []exec.Call) []string {
	out := make([]string, 0, len(calls))
	for _, c := range calls {
		out = append(out, c.String())
	}
	return out
}

// サービス制御は docs/api/external-interfaces.md に記載のコマンドを 1 本ずつ発行する。
//
// 監査ログの action と対象 runner が ctx に載っていること、読み取り専用の実行にしか
// 許されない SkipAudit が付いていないことも同じ表で見る。破壊的操作の記録漏れは
// 発行コマンドの誤りと同じ重さの欠陥である（docs/architecture/security.md）。
func TestControlIssuesDocumentedCommands(t *testing.T) {
	tests := map[string]struct {
		call   func(context.Context, exec.Executor, runner.Runner) error
		want   string
		action string
	}{
		"開始":      {Start, "systemctl start " + unitName, "svc.start"},
		"停止":      {Stop, "systemctl stop " + unitName, "svc.stop"},
		"再起動":     {Restart, "systemctl restart " + unitName, "svc.restart"},
		"enable":  {Enable, "systemctl enable " + unitName, "svc.enable"},
		"disable": {Disable, "systemctl disable " + unitName, "svc.disable"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			f := exec.NewFake()
			if err := tt.call(context.Background(), f, systemdRunner()); err != nil {
				t.Fatalf("err = %v", err)
			}

			calls := f.Calls()
			if len(calls) != 1 {
				t.Fatalf("発行コマンド = %q, want 1 本", cmdlines(calls))
			}
			if got := calls[0].String(); got != tt.want {
				t.Errorf("発行コマンド\n got: %s\nwant: %s", got, tt.want)
			}
			if o := calls[0].Options; o.Action != tt.action || o.Runner != "build01-1" {
				t.Errorf("監査メタ = %q/%q, want %q/%q", o.Action, o.Runner, tt.action, "build01-1")
			}
			if calls[0].Options.SkipAudit {
				t.Error("破壊的操作に SkipAudit が付いている")
			}
		})
	}
}

// daemon-reload はホスト全体に効くため runner を取らず、監査ログの runner も空になる。
func TestDaemonReloadIssuesCommand(t *testing.T) {
	f := exec.NewFake()
	if err := DaemonReload(context.Background(), f); err != nil {
		t.Fatalf("err = %v", err)
	}

	calls := f.Calls()
	if len(calls) != 1 || calls[0].String() != "systemctl daemon-reload" {
		t.Fatalf("発行コマンド = %q, want systemctl daemon-reload", cmdlines(calls))
	}
	if o := calls[0].Options; o.Action != "svc.daemon-reload" || o.Runner != "" || o.SkipAudit {
		t.Errorf("監査メタ = %q/%q/%v, want svc.daemon-reload/空/false", o.Action, o.Runner, o.SkipAudit)
	}
}

// ユニット名が分からない runner には systemctl を打たない。
//
// 打っても使い方のエラーで落ちるだけだが、「対象不明のまま制御を試みた」レコードが
// 監査ログに残り、利用者には終了コードしか返らない。呼ぶ前に理由を返す。
func TestControlRequiresUnitName(t *testing.T) {
	calls := map[string]func(context.Context, exec.Executor, runner.Runner) error{
		"開始": Start, "停止": Stop, "再起動": Restart, "enable": Enable, "disable": Disable,
	}
	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			f := exec.NewFake()
			err := call(context.Background(), f, standaloneRunner())
			if !errors.Is(err, ErrNoUnit) {
				t.Errorf("err = %v, want ErrNoUnit", err)
			}
			if got := f.Calls(); len(got) != 0 {
				t.Errorf("コマンドが発行されている: %q", cmdlines(got))
			}
		})
	}
}

// 終了コードが 0 でなければ失敗として返し、原因（標準エラー出力）を添える。
//
// 終了コードだけで失敗を表す Executor と、error でも返す Executor の両方を見る。
// どちらで表すかは実装によって変わるため、片方だけを見ると成功として素通りする。
func TestControlReportsFailure(t *testing.T) {
	t.Run("終了コードのみ", func(t *testing.T) {
		f := exec.NewFake()
		f.Push(exec.Result{ExitCode: 5, Stderr: []byte("Unit not found.\n")}, nil)

		err := Stop(context.Background(), f, systemdRunner())
		if err == nil {
			t.Fatal("非ゼロ終了が成功として扱われている")
		}
		for _, want := range []string{"systemctl stop " + unitName, "5", "Unit not found."} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("err = %q, %q を含まない", err, want)
			}
		}
	})

	t.Run("error で返る", func(t *testing.T) {
		want := errors.New("systemctl を起動できません")
		f := exec.NewFake()
		f.Push(exec.Result{ExitCode: -1}, want)

		err := Stop(context.Background(), f, systemdRunner())
		if !errors.Is(err, want) {
			t.Errorf("err = %v, want %v を包んだもの", err, want)
		}
	})
}

// 強制停止は kill -KILL を 1 回発行してから systemctl stop で systemd に停止を伝える。
//
// systemctl kill を使わないのはフラグの綴りが systemd のバージョンで変わり、既定では
// worker が生き残るためである（Kill の doc）。停止まで打つのは、プロセスを落としただけ
// では Restart= 付きのユニットが戻ってくるためである。
func TestKillSignalsProcessesThenStopsUnit(t *testing.T) {
	f := exec.NewFake()
	if err := Kill(context.Background(), f, runningRunner()); err != nil {
		t.Fatalf("err = %v", err)
	}

	want := []string{"kill -KILL 284102 284193", "systemctl stop " + unitName}
	if got := cmdlines(f.Calls()); !slices.Equal(got, want) {
		t.Fatalf("発行コマンド\n got: %q\nwant: %q", got, want)
	}
	for _, c := range f.Calls() {
		if c.Options.Action != "svc.kill" || c.Options.SkipAudit {
			t.Errorf("%s の監査メタ = %q/%v, want svc.kill/false", c, c.Options.Action, c.Options.SkipAudit)
		}
	}
}

// 強制停止は systemd 管理でなくても、プロセスが 1 つも見えていなくても成立する。
//
// screens.md は run.sh 直起動・判定不能のどちらでも X を塞がない。片方が欠けた状態で
// 打てなくなると、止める手段が UI から無くなる。
func TestKillWorksWithoutUnitOrProcesses(t *testing.T) {
	noUnit := runningRunner()
	noUnit.UnitName = ""

	noProcs := systemdRunner() // Listener も Workers も持たない

	tests := map[string]struct {
		runner runner.Runner
		want   []string
	}{
		"ユニットが無い": {noUnit, []string{"kill -KILL 284102 284193"}},
		"プロセスが無い": {noProcs, []string{"systemctl stop " + unitName}},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			f := exec.NewFake()
			if err := Kill(context.Background(), f, tt.runner); err != nil {
				t.Fatalf("err = %v", err)
			}
			if got := cmdlines(f.Calls()); !slices.Equal(got, tt.want) {
				t.Errorf("発行コマンド\n got: %q\nwant: %q", got, tt.want)
			}
		})
	}
}

// PID を取り損ねたプロセスには kill を送らない。
//
// kill(1) は 0 を「呼び出し元のプロセスグループ全員」と解釈するため、ゼロ値の PID を
// そのまま渡すと本ツール自身を巻き込む。対象が 1 つも残らなければ kill 自体を発行しない。
func TestKillSkipsUnusablePIDs(t *testing.T) {
	r := systemdRunner()
	r.Listener = &runner.Process{PID: 0, Kind: runner.ProcListener, Dir: r.Dir}
	r.Workers = []runner.Process{
		{PID: 1, Kind: runner.ProcWorker, Dir: r.Dir},
		{PID: 284193, Kind: runner.ProcWorker, Dir: r.Dir},
	}

	f := exec.NewFake()
	if err := Kill(context.Background(), f, r); err != nil {
		t.Fatalf("err = %v", err)
	}
	want := []string{"kill -KILL 284193", "systemctl stop " + unitName}
	if got := cmdlines(f.Calls()); !slices.Equal(got, want) {
		t.Errorf("発行コマンド\n got: %q\nwant: %q", got, want)
	}

	r.Workers = nil
	f.Reset()
	if err := Kill(context.Background(), f, r); err != nil {
		t.Fatalf("err = %v", err)
	}
	if got := cmdlines(f.Calls()); !slices.Equal(got, []string{"systemctl stop " + unitName}) {
		t.Errorf("使える PID が無いのに kill を発行している: %q", got)
	}
}

// kill が失敗しても停止は試み、両方の失敗をまとめて返す。
//
// 片方で打ち切ると、残った方（プロセス or ユニット）が黙って生き残る。
func TestKillReportsBothFailures(t *testing.T) {
	killErr := errors.New("kill に失敗しました")
	stopErr := errors.New("stop に失敗しました")

	f := exec.NewFake()
	f.SetFunc(func(name string, _ []string) (exec.Result, error) {
		if name == "kill" {
			return exec.Result{ExitCode: 1}, killErr
		}
		return exec.Result{ExitCode: 1}, stopErr
	})

	err := Kill(context.Background(), f, runningRunner())
	if !errors.Is(err, killErr) || !errors.Is(err, stopErr) {
		t.Errorf("err = %v, want kill と stop の両方を含む", err)
	}
	if got := len(f.Calls()); got != 2 {
		t.Errorf("発行コマンド = %d 本, want 2 本（kill の失敗で停止を諦めている）", got)
	}
}
