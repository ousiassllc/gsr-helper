package logcheck_test

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/ousiassllc/gsr-helper/internal/logs"
	"github.com/ousiassllc/gsr-helper/internal/runner"
)

// `_diag` は runner 実行ユーザーが書き換えられるので、ログ名の FIFO やデバイスへの
// リンクが置かれうる（Issue #210）。これらを一覧に載せず、開いても固まらない
// ことを確かめる。FIFO や /dev/zero を読んでしまう実装は、待ちが返らないため
// 上限付きの待ちで落ちる（固定時間の Sleep は使わない）。

// timeout は、固まらずに返るはずの処理を待つ上限。
const timeout = 3 * time.Second

// mkfifo は path に FIFO を作る。落ちたときに open(2) で止まったままの goroutine を
// 解放できるよう、後始末で書き手側（O_RDWR）を開いて閉じる。
func mkfifo(t *testing.T, path string) {
	t.Helper()
	if err := syscall.Mkfifo(path, 0o600); err != nil {
		t.Fatalf("FIFO を作れない: %v", err)
	}
	t.Cleanup(func() {
		if f, err := os.OpenFile(path, os.O_RDWR, 0); err == nil {
			_ = f.Close()
		}
	})
}

// symlinkZero は path に /dev/zero へのリンクを作る。
func symlinkZero(t *testing.T, path string) {
	t.Helper()
	if err := os.Symlink("/dev/zero", path); err != nil {
		t.Fatalf("リンクを作れない: %v", err)
	}
}

// within は fn が timeout 内に返ることを確かめる。
func within(t *testing.T, what string, fn func()) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		defer close(done)
		fn()
	}()
	select {
	case <-done:
	case <-time.After(timeout):
		t.Fatalf("%s が返らない（FIFO やデバイスを読んで固まっている）", what)
	}
}

// 通常ファイル以外は一覧に載せない。
func TestListSkipsNonRegularFiles(t *testing.T) {
	dir := t.TempDir()
	diag := filepath.Join(dir, logs.DiagDir)
	if err := os.MkdirAll(diag, 0o755); err != nil {
		t.Fatalf("_diag を作れない: %v", err)
	}
	good := filepath.Join(diag, "Runner_1.log")
	if err := os.WriteFile(good, []byte("a\n"), 0o600); err != nil {
		t.Fatalf("ログを書けない: %v", err)
	}
	mkfifo(t, filepath.Join(diag, "Runner_2.log"))
	mkfifo(t, filepath.Join(diag, "Worker_2.log"))
	symlinkZero(t, filepath.Join(diag, "Runner_3.log"))
	// 通常ファイルを指すリンクも載せない（開く段が O_NOFOLLOW で拒むのと揃える）。
	if err := os.Symlink(good, filepath.Join(diag, "Worker_3.log")); err != nil {
		t.Fatalf("リンクを作れない: %v", err)
	}

	r := runner.Runner{}
	r.Dir = dir
	got, err := logs.List(r)
	if err != nil {
		t.Fatalf("List が失敗した: %v", err)
	}
	if len(got) != 1 || got[0].Name != "Runner_1.log" {
		t.Errorf("一覧 = %v, want Runner_1.log の 1 件だけ", got)
	}
}

// FIFO・デバイスへのリンクの Worker ログは、解析を固めず空を返す。
func TestParseWorkerNonRegularReturnsEmpty(t *testing.T) {
	cases := map[string]func(*testing.T, string){
		"FIFO":      mkfifo,
		"/dev/zero": symlinkZero,
	}
	for name, mk := range cases {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			mk(t, filepath.Join(dir, "Worker_1.log"))

			var got logs.JobInfo
			var err error
			within(t, "ParseWorker", func() { got, err = logs.ParseWorker(dir, "Worker_1.log") })
			if err != nil || got != (logs.JobInfo{}) {
				t.Errorf("ParseWorker = %+v, %v, want 空の JobInfo と nil", got, err)
			}
		})
	}
}

// FIFO・デバイスへのリンクを Tail に直接渡しても、固まらずにエラーで返り、チャネルを閉じる。
func TestTailRejectsNonRegularFile(t *testing.T) {
	cases := map[string]func(*testing.T, string){
		"FIFO":      mkfifo,
		"/dev/zero": symlinkZero,
	}
	for name, mk := range cases {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "Runner_1.log")
			mk(t, path)

			ch := make(chan logs.Line, 1)
			var err error
			within(t, "Tail", func() { err = logs.Tail(t.Context(), path, ch) })
			if err == nil {
				t.Error("通常ファイルでないログでエラーにならなかった")
			}
			if _, ok := <-ch; ok {
				t.Error("チャネルが閉じられていない")
			}
		})
	}
}
