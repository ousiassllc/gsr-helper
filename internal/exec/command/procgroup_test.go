package command

import (
	"context"
	"errors"
	"os"
	osexec "os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/ousiassllc/gsr-helper/internal/exec"
)

// processGone は pid のプロセスが存在しなくなったかを返す。
// シグナル 0 は送信せず存在確認だけを行う（kill(2) の定石）。
func processGone(pid int) bool {
	return errors.Is(syscall.Kill(pid, 0), syscall.ESRCH)
}

func TestCommandRunKillsProcessGroupOnTimeout(t *testing.T) {
	// runner のスクリプトは sudo や systemctl を起動するため、中断が直接の子だけに
	// 届くと孫が runner ディレクトリを掴んだまま孤児として残る。孫まで止まることを
	// 固定する。
	name, args := helperCommand()
	ctx := exec.WithOptions(context.Background(), exec.Options{
		Action: "test.grandchild",
		Env:    helperEnv(helperModeEnv + "=grandchild"),
	})

	res, err := New(NoSecrets, WithTimeout(300*time.Millisecond)).Run(ctx, name, args...)
	if err == nil {
		t.Fatal("タイムアウトしたのにエラーを返していない")
	}

	pid, convErr := strconv.Atoi(strings.TrimSpace(string(res.Stdout)))
	if convErr != nil {
		t.Fatalf("孫プロセスの PID が読めない: %v (stdout=%q, stderr=%q)", convErr, res.Stdout, res.Stderr)
	}
	// 判定が失敗しても野良プロセスを残さない。
	defer func() { _ = syscall.Kill(pid, syscall.SIGKILL) }()

	// 終了の観測には僅かな猶予が要るため、上限を決めて待つ。
	deadline := time.Now().Add(2 * time.Second)
	for !processGone(pid) {
		if time.Now().After(deadline) {
			t.Fatalf("孫プロセス %d がタイムアウト後も残っている", pid)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestKillProcessGroupOnFinishedProcess(t *testing.T) {
	// 中断とプロセスの自然終了が競合しただけなので偽のエラーにしてはならない。
	// os.ErrProcessDone を返すのは既定の Cancel（Process.Kill）と同じ扱いにするため。
	name, args := helperCommand()
	cmd := osexec.Command(name, args...)
	cmd.Env = append(os.Environ(), helperEnv()...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Run(); err != nil {
		t.Fatalf("ヘルパープロセスの実行に失敗した: %v", err)
	}

	// 終了して回収済みなので、プロセスグループ全体が存在しない（ESRCH）。
	if err := killProcessGroup(cmd.Process); !errors.Is(err, os.ErrProcessDone) {
		t.Errorf("終了済みプロセスに対する返り値 = %v, want os.ErrProcessDone", err)
	}
	if err := killProcessGroup(nil); err != nil {
		t.Errorf("未起動（Process が nil）でエラーを返した: %v", err)
	}
}
