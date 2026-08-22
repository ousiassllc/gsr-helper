package command

import (
	"fmt"
	"io"
	"os"
	osexec "os/exec"
	"strconv"
	"strings"
	"testing"
	"time"
)

// ヘルパープロセスの振る舞いを指定する環境変数。
//
// /bin/sh や sleep のような外部コマンドに依存すると、環境によってテストが
// 落ちたり挙動が変わったりする。代わりにテストバイナリ自身を子として起動し、
// 終了コード・標準出力・標準エラー出力・遅延をここで制御する。
const (
	helperProcessEnv = "GSR_HELPER_TEST_PROCESS"
	helperStdoutEnv  = "GSR_HELPER_TEST_STDOUT"
	helperStderrEnv  = "GSR_HELPER_TEST_STDERR"
	helperExitEnv    = "GSR_HELPER_TEST_EXIT"
	helperSleepEnv   = "GSR_HELPER_TEST_SLEEP_MS"
	helperModeEnv    = "GSR_HELPER_TEST_MODE"
	helperEchoEnv    = "GSR_HELPER_TEST_ECHO_ENV"
	helperStderrKiB  = "GSR_HELPER_TEST_STDERR_KIB"
)

// helperCommand はテストバイナリ自身をヘルパープロセスとして起動する
// コマンド名と引数を返す。
//
// 追加の引数は "--" の後ろに置く。testing のフラグ解析は "--" で止まるため、
// マスク検証用の --token のような引数を渡してもヘルパー側が起動に失敗しない。
func helperCommand(args ...string) (string, []string) {
	return os.Args[0], append([]string{"-test.run=^TestHelperProcess$", "--"}, args...)
}

// helperEnv はヘルパープロセスを有効にする環境変数列を組み立てる。
func helperEnv(kv ...string) []string {
	return append([]string{helperProcessEnv + "=1"}, kv...)
}

// TestHelperProcess は子プロセスとして起動されたときの振る舞い。
// 通常のテスト実行ではスキップされる。
func TestHelperProcess(t *testing.T) {
	if os.Getenv(helperProcessEnv) != "1" {
		t.Skip("ヘルパープロセスとして起動されたときのみ実行する")
	}

	if ms, err := strconv.Atoi(os.Getenv(helperSleepEnv)); err == nil && ms > 0 {
		time.Sleep(time.Duration(ms) * time.Millisecond)
	}

	switch os.Getenv(helperModeEnv) {
	case "cwd":
		wd, err := os.Getwd()
		if err != nil {
			fmt.Fprint(os.Stderr, err)
			os.Exit(2)
		}
		fmt.Fprint(os.Stdout, wd)
	case "stdin":
		// 標準入力が与えられていなければ即 EOF になり、ここで止まらない。
		b, err := io.ReadAll(os.Stdin)
		if err != nil {
			fmt.Fprint(os.Stderr, err)
			os.Exit(2)
		}
		fmt.Fprintf(os.Stdout, "stdin=%d", len(b))
	case "env":
		fmt.Fprint(os.Stdout, os.Getenv(os.Getenv(helperEchoEnv)))
	case "grandchild":
		helperSpawnGrandchild(t)
	case "stderrflood":
		// 指定された KiB ぶんの標準エラー出力を吐き、最後に印を置く。上限が効いて
		// いないと、この出力がそのまま監査ログの 1 行と ExitError に載る。
		kib, _ := strconv.Atoi(os.Getenv(helperStderrKiB))
		for range kib {
			fmt.Fprintln(os.Stderr, strings.Repeat("E", 1023))
		}
		fmt.Fprint(os.Stderr, stderrFloodTail)
	default:
		fmt.Fprint(os.Stdout, os.Getenv(helperStdoutEnv))
		fmt.Fprint(os.Stderr, os.Getenv(helperStderrEnv))
	}

	code, _ := strconv.Atoi(os.Getenv(helperExitEnv))
	os.Exit(code)
}

// helperSpawnGrandchild は孫プロセスを 1 つ起こし、その PID を標準出力へ出したうえで
// 自分も生き続ける。中断が直接の子だけに届く実装では、この孫が孤児として残る。
func helperSpawnGrandchild(t *testing.T) {
	t.Helper()

	name, args := helperCommand()
	child := osexec.Command(name, args...)
	// mode を空に上書きする。そのまま継承すると孫がさらに孫を起こす連鎖になる。
	// 重複したキーは後の値が勝つため、os.Environ() の後ろに置けばよい。
	child.Env = append(os.Environ(), helperModeEnv+"=", helperExitEnv+"=0", helperSleepEnv+"=600000")
	if err := child.Start(); err != nil {
		fmt.Fprint(os.Stderr, err)
		os.Exit(2)
	}

	// PID を先に出す。テストはこれを見て孫の生死を確かめる。
	fmt.Fprintln(os.Stdout, child.Process.Pid)
	// 親も残す。親だけが kill されたときに孫が残ることを観測するためである。
	time.Sleep(10 * time.Minute)
}
