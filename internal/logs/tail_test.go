package logs

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// tailTimeout は 1 行を待つ上限。追記の検知が届かない不具合を無限待ちにしない。
const tailTimeout = 3 * time.Second

// startTail は追従を開始し、行のチャネルと停止関数を返す。
func startTail(t *testing.T, path string) (<-chan Line, context.CancelFunc) {
	t.Helper()

	ctx, cancel := context.WithCancel(t.Context())
	ch := make(chan Line, 64)
	errc := make(chan error, 1)
	go func() { errc <- Tail(ctx, path, ch) }()
	t.Cleanup(func() {
		cancel()
		if err := <-errc; err != nil {
			t.Errorf("Tail が失敗した: %v", err)
		}
	})
	return ch, cancel
}

// next は次の 1 行を待つ。届かなければ失敗させる。
func next(t *testing.T, ch <-chan Line) Line {
	t.Helper()

	select {
	case l, ok := <-ch:
		if !ok {
			t.Fatal("チャネルが閉じられた（行が届く前に追従が終わった）")
		}
		return l
	case <-time.After(tailTimeout):
		t.Fatal("行が届かない")
		return Line{}
	}
}

// append は既存のログへ追記する。
func appendLine(t *testing.T, path, s string) {
	t.Helper()

	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatalf("追記できない: %v", err)
	}
	defer func() { _ = f.Close() }()
	if _, err := f.WriteString(s); err != nil {
		t.Fatalf("追記できない: %v", err)
	}
}

// 既存の行を送ったあと、追記を検知して送る（FR-24）。
func TestTailSendsExistingThenAppended(t *testing.T) {
	path := writeLog(t, t.TempDir(), "Worker_1.log", "first\n", time.Now())
	ch, _ := startTail(t, path)

	if got := next(t, ch); got.Text != "first" {
		t.Fatalf("1 行目 = %q, want first", got.Text)
	}
	appendLine(t, path, "[ERROR] second\n")
	got := next(t, ch)
	if got.Text != "[ERROR] second" {
		t.Errorf("2 行目 = %q, want [ERROR] second", got.Text)
	}
	if got.Level != LevelError {
		t.Errorf("2 行目の重大度 = %v, want LevelError", got.Level)
	}
}

// 改行で終わっていない末尾は、残りが届いてから 1 行として送る。
func TestTailWaitsForCompleteLine(t *testing.T) {
	path := writeLog(t, t.TempDir(), "Worker_1.log", "", time.Now())
	ch, _ := startTail(t, path)

	appendLine(t, path, "half")
	appendLine(t, path, "-line\n")
	if got := next(t, ch); got.Text != "half-line" {
		t.Errorf("行 = %q, want half-line（分割して届いた 1 行）", got.Text)
	}
}

// 切り詰められたら先頭から読み直す（位置を保つと存在しない領域を読み続ける）。
func TestTailRestartsAfterTruncate(t *testing.T) {
	path := writeLog(t, t.TempDir(), "Worker_1.log", "old line\n", time.Now())
	ch, _ := startTail(t, path)
	if got := next(t, ch); got.Text != "old line" {
		t.Fatalf("1 行目 = %q, want old line", got.Text)
	}

	if err := os.WriteFile(path, []byte("fresh\n"), 0o600); err != nil {
		t.Fatalf("切り詰められない: %v", err)
	}
	if got := next(t, ch); got.Text != "fresh" {
		t.Errorf("切り詰め後の行 = %q, want fresh", got.Text)
	}
}

// 大きなログは末尾だけを読み、途中から始まる断片は出さない。
func TestTailReadsOnlyTailOfLargeFile(t *testing.T) {
	dir := t.TempDir()
	body := strings.Repeat("x", 1000) + "\n"
	var b strings.Builder
	for b.Len() < TailInitialBytes+2*len(body) {
		b.WriteString(body)
	}
	b.WriteString("last line\n")
	path := writeLog(t, dir, "Runner_1.log", b.String(), time.Now())

	ch, _ := startTail(t, path)
	first := next(t, ch)
	if len(first.Text) != 1000 {
		t.Errorf("最初の行の長さ = %d, want 1000（行の途中から始まっている）", len(first.Text))
	}
}

// ctx をキャンセルするとチャネルを閉じて終わる（購読の終わりを受け手が知る）。
func TestTailClosesChannelOnCancel(t *testing.T) {
	path := writeLog(t, t.TempDir(), "Worker_1.log", "a\n", time.Now())
	ctx, cancel := context.WithCancel(t.Context())
	ch := make(chan Line, 4)
	done := make(chan error, 1)
	go func() { done <- Tail(ctx, path, ch) }()

	if got := next(t, ch); got.Text != "a" {
		t.Fatalf("1 行目 = %q, want a", got.Text)
	}
	cancel()
	if err := <-done; err != nil {
		t.Errorf("キャンセルで失敗した: %v", err)
	}
	if _, ok := <-ch; ok {
		t.Error("チャネルが閉じられていない")
	}
}

// 開けないログは理由を返す（存在しないファイルを黙って空表示にしない）。
func TestTailMissingFile(t *testing.T) {
	ch := make(chan Line, 1)
	err := Tail(t.Context(), filepath.Join(t.TempDir(), "none.log"), ch)
	if err == nil {
		t.Fatal("存在しないログでエラーにならなかった")
	}
	if _, ok := <-ch; ok {
		t.Error("チャネルが閉じられていない")
	}
}
