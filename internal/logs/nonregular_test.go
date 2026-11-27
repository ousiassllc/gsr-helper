package logs

import (
	"bufio"
	"context"
	"errors"
	"os"
	"syscall"
	"testing"
	"time"
)

// FIFO や無限の入力に対する内部経路（reopen / readLine）の検査。公開 API 越しの
// 検査は logcheck にある。固まる実装は上限付きの待ちで落ちる（固定時間の Sleep は使わない）。

// nonRegularTimeout は、固まらずに返るはずの処理を待つ上限。
const nonRegularTimeout = 3 * time.Second

// 追従中に同名で FIFO へ差し替えられても、開き直しで固まらず、いまのファイルを保つ。
func TestReopenKeepsFileWhenReplacedByFIFO(t *testing.T) {
	path := writeLog(t, t.TempDir(), "Runner_1.log", "a\n", time.Now())
	f, err := openRegular(path)
	if err != nil {
		t.Fatalf("開けない: %v", err)
	}
	t.Cleanup(func() { _ = f.Close() })
	if err := os.Remove(path); err != nil {
		t.Fatalf("ログを消せない: %v", err)
	}
	if err := syscall.Mkfifo(path, 0o600); err != nil {
		t.Fatalf("FIFO を作れない: %v", err)
	}
	// 落ちたときに open(2) で止まったままの goroutine を解放する。
	t.Cleanup(func() {
		if w, err := os.OpenFile(path, os.O_RDWR, 0); err == nil {
			_ = w.Close()
		}
	})

	var got *os.File
	var off int64
	done := make(chan struct{})
	go func() {
		defer close(done)
		got, off = reopen(f, path, 2)
	}()
	select {
	case <-done:
	case <-time.After(nonRegularTimeout):
		t.Fatal("reopen が返らない（FIFO を開いて固まっている）")
	}
	if got != f || off != 2 {
		t.Errorf("reopen = %p, %d, want 元のファイルと位置 2", got, off)
	}
}

// ctxReader は after 回読まれたら cancel を呼ぶ、改行の来ない無限の入力。
type ctxReader struct {
	reads  int
	after  int
	cancel context.CancelFunc
}

func (r *ctxReader) Read(p []byte) (int, error) {
	r.reads++
	if r.reads == r.after {
		r.cancel()
	}
	clear(p)
	return len(p), nil
}

// 改行の来ない入力でも、readLine は ctx のキャンセルで抜ける。
func TestReadLineStopsOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	src := &ctxReader{after: 3, cancel: cancel}

	var err error
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _, _, err = readLine(ctx, bufio.NewReaderSize(src, readBufBytes))
	}()
	select {
	case <-done:
	case <-time.After(nonRegularTimeout):
		t.Fatal("readLine が返らない（キャンセルを見ていない）")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("readLine の err = %v, want context.Canceled", err)
	}
}
