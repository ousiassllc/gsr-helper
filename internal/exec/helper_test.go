package exec

import (
	"fmt"
	"io"
	"os"
	"strconv"
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
	default:
		fmt.Fprint(os.Stdout, os.Getenv(helperStdoutEnv))
		fmt.Fprint(os.Stderr, os.Getenv(helperStderrEnv))
	}

	code, _ := strconv.Atoi(os.Getenv(helperExitEnv))
	os.Exit(code)
}
