package buildconfig

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// makeTimeout は make の実行を打ち切るまでの時間。対象 0 件で gofmt が標準入力を
// 読み始めた場合（ハング）をテストとして検知するために使う。
const makeTimeout = 60 * time.Second

// repoRoot はテスト実行ディレクトリから遡り、go.mod を持つリポジトリルートを返す。
func repoRoot(t *testing.T) string {
	t.Helper()

	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("カレントディレクトリを取得できない: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod を持つリポジトリルートが見つからない")
		}
		dir = parent
	}
}

// writeFiles は dir 配下に、相対パスをキーとするファイルを書き出す。
func writeFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()

	for name, body := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("%s の親ディレクトリを作成できない: %v", name, err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatalf("%s を書き出せない: %v", name, err)
		}
	}
}

// newModule は独立した Go モジュールを一時ディレクトリに作る。files に "go.mod" が
// 含まれる場合はその内容が優先される（go.mod 破損時の挙動を検証するため）。
func newModule(t *testing.T, files map[string]string) string {
	t.Helper()

	dir := t.TempDir()
	if _, ok := files["go.mod"]; !ok {
		writeFiles(t, dir, map[string]string{"go.mod": "module example.com/fixture\n\ngo 1.24\n"})
	}
	writeFiles(t, dir, files)
	return dir
}

// runMake はリポジトリルートの Makefile を dir 上で実行し、出力と終了コードを返す。
//
// 標準入力にはデータの来ないパイプを渡す。対象ファイル 0 件で gofmt が引数なしに
// 起動すると標準入力を読んで無限に待つため、その退行を makeTimeout で検知できる。
func runMake(t *testing.T, dir string, extraEnv []string, args ...string) (string, int) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), makeTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "make", append([]string{"-f", filepath.Join(repoRoot(t), "Makefile")}, args...)...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), extraEnv...)
	cmd.WaitDelay = time.Second

	stdin, stdinWriter, err := os.Pipe()
	if err != nil {
		t.Fatalf("標準入力用のパイプを作成できない: %v", err)
	}
	defer stdinWriter.Close()
	cmd.Stdin = stdin

	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	runErr := cmd.Run()
	stdin.Close()

	if ctx.Err() != nil {
		t.Fatalf("make %v が %s 以内に終了しなかった（標準入力待ちの可能性がある）\n出力:\n%s", args, makeTimeout, out.String())
	}

	var exitErr *exec.ExitError
	switch {
	case runErr == nil:
		return out.String(), 0
	case errors.As(runErr, &exitErr):
		return out.String(), exitErr.ExitCode()
	default:
		t.Fatalf("make %v を実行できない: %v", args, runErr)
		return "", 0
	}
}

// goEnv は go env の値を 1 つ取得する。
func goEnv(t *testing.T, name string) string {
	t.Helper()

	out, err := exec.Command("go", "env", name).Output()
	if err != nil {
		t.Fatalf("go env %s に失敗した: %v", name, err)
	}
	return string(bytes.TrimSpace(out))
}
