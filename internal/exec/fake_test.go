package exec

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"
)

// 呼び出し側がタイムアウトを課しているかを Fake だけで検証できることを確かめる。
func TestFakeRecordsDeadline(t *testing.T) {
	f := NewFake()

	if _, err := f.Run(context.Background(), "systemctl", "stop"); err != nil {
		t.Fatalf("Run がエラーを返した: %v", err)
	}
	if got := f.Calls()[0]; got.HasDeadline {
		t.Errorf("deadline 無しの ctx で HasDeadline = true, Deadline = %v", got.Deadline)
	}

	const timeout = 30 * time.Second
	before := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if _, err := f.Run(ctx, "systemctl", "stop"); err != nil {
		t.Fatalf("Run がエラーを返した: %v", err)
	}

	got := f.Calls()[1]
	if !got.HasDeadline {
		t.Fatal("WithTimeout を掛けた ctx で HasDeadline = false")
	}
	// 経過時間ぶんのずれを許容し、期待した timeout の範囲に収まることだけを見る。
	if got.Deadline.Before(before.Add(timeout)) || got.Deadline.After(time.Now().Add(timeout)) {
		t.Errorf("Deadline = %v, want %v 付近", got.Deadline, before.Add(timeout))
	}
}

func TestFakeRecordsCalls(t *testing.T) {
	f := NewFake()
	ctx := WithOptions(context.Background(), Options{
		Action: "svc.stop",
		Runner: "build01-2",
		Dir:    "/opt/runners/build01-2",
	})

	if _, err := f.Run(ctx, "systemctl", "stop", "actions.runner.foo.build01-2.service"); err != nil {
		t.Fatalf("Run がエラーを返した: %v", err)
	}

	calls := f.Calls()
	if len(calls) != 1 {
		t.Fatalf("記録件数 = %d, want 1", len(calls))
	}
	if calls[0].Name != "systemctl" {
		t.Errorf("Name = %q, want systemctl", calls[0].Name)
	}
	if want := []string{"stop", "actions.runner.foo.build01-2.service"}; !slices.Equal(calls[0].Args, want) {
		t.Errorf("Args = %q, want %q", calls[0].Args, want)
	}
	if calls[0].Options.Action != "svc.stop" || calls[0].Options.Runner != "build01-2" {
		t.Errorf("Options = %+v", calls[0].Options)
	}
	if want := "systemctl stop actions.runner.foo.build01-2.service"; calls[0].String() != want {
		t.Errorf("String() = %q, want %q", calls[0].String(), want)
	}
}

func TestFakePushIsFIFO(t *testing.T) {
	f := NewFake()
	wantErr := errors.New("2 番目は失敗")
	f.Push(Result{Stdout: []byte("first")}, nil)
	f.Push(Result{ExitCode: 1}, wantErr)

	res, err := f.Run(context.Background(), "a")
	if err != nil || string(res.Stdout) != "first" {
		t.Fatalf("1 回目 = (%q, %v), want (first, nil)", res.Stdout, err)
	}

	res, err = f.Run(context.Background(), "b")
	if !errors.Is(err, wantErr) || res.ExitCode != 1 {
		t.Fatalf("2 回目 = (%d, %v), want (1, %v)", res.ExitCode, err, wantErr)
	}

	// 積んだ応答を使い切った後は成功として振る舞う。
	res, err = f.Run(context.Background(), "c")
	if err != nil || res.ExitCode != 0 || len(res.Stdout) != 0 {
		t.Fatalf("3 回目 = (%+v, %v), want ゼロ値", res, err)
	}
}

func TestFakeSetFunc(t *testing.T) {
	f := NewFake()
	f.Push(Result{Stdout: []byte("push")}, nil)
	f.SetFunc(func(name string, args []string) (Result, error) {
		return Result{Stdout: []byte(name + ":" + args[0])}, nil
	})

	res, err := f.Run(context.Background(), "systemctl", "stop")
	if err != nil {
		t.Fatalf("Run がエラーを返した: %v", err)
	}
	if got := string(res.Stdout); got != "systemctl:stop" {
		t.Errorf("Stdout = %q, want systemctl:stop（SetFunc が優先されていない）", got)
	}
}

func TestFakeRespectsCanceledContext(t *testing.T) {
	f := NewFake()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	res, err := f.Run(ctx, "systemctl", "stop")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("context.Canceled を包んだエラーになっていない: %v", err)
	}
	if res.ExitCode != -1 {
		t.Errorf("ExitCode = %d, want -1", res.ExitCode)
	}
	// 何を実行しようとしたかは記録しておく。
	if len(f.Calls()) != 1 {
		t.Errorf("記録件数 = %d, want 1", len(f.Calls()))
	}
}

func TestFakeCallsReturnsCopy(t *testing.T) {
	f := NewFake()
	ctx, cancel := context.WithTimeout(WithOptions(context.Background(), Options{Env: []string{"A=1"}}), time.Minute)
	defer cancel()
	if _, err := f.Run(ctx, "systemctl", "stop"); err != nil {
		t.Fatalf("Run がエラーを返した: %v", err)
	}

	calls := f.Calls()
	want := calls[0].Deadline
	calls[0].Name = "changed"
	calls[0].Args[0] = "changed"
	calls[0].Options.Env[0] = "changed"
	calls[0].Deadline = time.Time{}
	calls[0].HasDeadline = false
	calls = append(calls, Call{Name: "extra"})
	_ = calls

	again := f.Calls()
	if len(again) != 1 {
		t.Fatalf("記録件数 = %d, want 1", len(again))
	}
	if again[0].Name != "systemctl" || again[0].Args[0] != "stop" || again[0].Options.Env[0] != "A=1" {
		t.Errorf("返した値の変更が内部に伝わっている: %+v", again[0])
	}
	if !again[0].HasDeadline || !again[0].Deadline.Equal(want) {
		t.Errorf("Deadline = (%v, %t), want (%v, true)", again[0].Deadline, again[0].HasDeadline, want)
	}
}

func TestFakeReset(t *testing.T) {
	f := NewFake()
	f.Push(Result{Stdout: []byte("pushed")}, nil)
	f.SetFunc(func(string, []string) (Result, error) { return Result{ExitCode: 9}, nil })
	if _, err := f.Run(context.Background(), "systemctl"); err != nil {
		t.Fatalf("Run がエラーを返した: %v", err)
	}

	f.Reset()
	if len(f.Calls()) != 0 {
		t.Errorf("記録が残っている: %+v", f.Calls())
	}

	res, err := f.Run(context.Background(), "systemctl")
	if err != nil || res.ExitCode != 0 || len(res.Stdout) != 0 {
		t.Errorf("Reset 後の応答 = (%+v, %v), want ゼロ値", res, err)
	}
}
