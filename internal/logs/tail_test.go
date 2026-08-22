package logs

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
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
//
// 行に通し番号を埋めるのは、「先頭から全部読む」実装との差を見えるようにするため。
// 全行が同じ内容だと、届いた行の長さしか見られず、遡る量を無視して先頭から
// 読んでもテストが緑のままになる。番号と受信行数の両方を見て、末尾側から
// 始まっていることを確かめる。
func TestTailReadsOnlyTailOfLargeFile(t *testing.T) {
	const lineLen = 1000
	// 遡る量（TailInitialBytes）を確実に超える行数を書く。20 行の上積みは、
	// 遡った先が行の途中になっても捨てる行が残るようにするための余裕。
	total := TailInitialBytes/(lineLen+1) + 20
	var b strings.Builder
	for i := range total {
		fmt.Fprintf(&b, "%04d %s\n", i, strings.Repeat("x", lineLen-5))
	}
	b.WriteString("last line\n")
	path := writeLog(t, t.TempDir(), "Runner_1.log", b.String(), time.Now())

	ch, _ := startTail(t, path)
	first := next(t, ch)
	if len(first.Text) != lineLen {
		t.Fatalf("最初の行の長さ = %d, want %d（行の途中から始まっている）", len(first.Text), lineLen)
	}
	firstNo, err := strconv.Atoi(first.Text[:4])
	if err != nil {
		t.Fatalf("最初の行 %q から通し番号を読めない: %v", first.Text[:8], err)
	}
	if firstNo == 0 {
		t.Errorf("最初に届いた行 = %d 行目, want 末尾側の行（先頭から全部読んでいる）", firstNo)
	}

	// 末尾まで受け取り、行数が「遡る量に収まる」ことを見る。番号だけだと、
	// 先頭から読みつつ最初の 1 行を捨てる実装を素通しさせてしまう。
	got := 1
	for l := first; l.Text != "last line"; got++ {
		l = next(t, ch)
	}
	if want := TailInitialBytes/(lineLen+1) + 2; got > want {
		t.Errorf("受信した行数 = %d, want %d 以下（遡る量を超えて読んでいる）", got, want)
	}
}

// 改行を含まない長大な行は maxLineBytes で打ち切り、次の行はずれずに届く。
//
// 打ち切るのは本文だけで、消費したバイト数は打ち切らない（readLine の契約）。
// 両方を打ち切ってしまうと読み出し位置が取り残され、以後の行が 1 つずつ
// ずれて届く。だから「長さの上限」と「次の行が正しいこと」を並べて見る。
func TestTailTruncatesOverlongLine(t *testing.T) {
	// バッファ 1 杯（readBufBytes）を超えて読み継ぐ経路を通すため、上限より
	// 十分に長い 1 行を書く。
	overlong := strings.Repeat("y", maxLineBytes+4096)
	path := writeLog(t, t.TempDir(), "Runner_1.log", overlong+"\nnext line\n", time.Now())

	ch, _ := startTail(t, path)
	if got := next(t, ch); len(got.Text) != maxLineBytes {
		t.Errorf("長大な行の長さ = %d, want %d（打ち切られていない）", len(got.Text), maxLineBytes)
	}
	if got := next(t, ch); got.Text != "next line" {
		t.Errorf("次の行 = %q, want next line（読み出し位置がずれている）", got.Text)
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
