package command

import (
	"bytes"
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/ousiassllc/gsr-helper/internal/exec"
	"github.com/ousiassllc/gsr-helper/internal/exec/command/limit"
)

const (
	// stderrFloodTail は stderrflood モードが最後に出す印。
	// 「原因は末尾に出る」という前提どおり、抜粋にここが残ることを確かめる。
	stderrFloodTail = "最後の行: ここに失敗の原因が出る"
	// floodOverExcerptKiB は抜粋上限（4 KiB）は超えるが取り込み上限（1 MiB）には
	// 収まる量。取り込みで落ちていない状態で末尾が残ることを検証するために使う。
	floodOverExcerptKiB = 64
	// floodOverCaptureKiB は取り込み上限（1 MiB）を大きく超える量。
	floodOverCaptureKiB = 3 * 1024
)

// runStderrFlood は kib KiB の ASCII の標準エラー出力を吐いて非ゼロで終わる
// コマンドを実行し、監査ログの内容とともに返す。
func runStderrFlood(t *testing.T, kib int) (exec.Result, *bytes.Buffer, error) {
	t.Helper()

	return runStderrFloodFill(t, kib, "")
}

// runStderrFloodFill は 1 行の埋め草を選べる runStderrFlood。空文字なら ASCII。
func runStderrFloodFill(t *testing.T, kib int, fill string) (exec.Result, *bytes.Buffer, error) {
	t.Helper()

	var buf bytes.Buffer
	c := New(NoSecrets, WithAudit(testLogger(&buf)))

	name, args := helperCommand()
	ctx := exec.WithOptions(context.Background(), exec.Options{
		Action: "test.flood",
		Env: helperEnv(helperModeEnv+"=stderrflood", helperExitEnv+"=3",
			helperStderrKiB+"="+strconv.Itoa(kib), helperStderrFill+"="+fill),
	})

	res, err := c.Run(ctx, name, args...)
	if err == nil {
		t.Fatal("非ゼロ終了でエラーを返していない")
	}
	return res, &buf, err
}

func TestCommandRunDoesNotRecordStderrInAudit(t *testing.T) {
	// 監査ログの 1 行は grep できる大きさに収まり、かつコマンドの標準エラー出力を
	// 一切含まない（値一致マスクでは救えない資格情報がディスクに残るため）。
	_, buf, err := runStderrFlood(t, floodOverCaptureKiB)

	if buf.Len() >= 8<<10 {
		t.Errorf("監査ログ 1 行が %d バイトある（上限が効いていない）", buf.Len())
	}

	rec := decodeAudit(t, buf)
	if !strings.Contains(rec.Error, "終了コード 3") {
		t.Errorf("error に終了コードが記録されていない: %q", rec.Error)
	}
	if strings.Contains(rec.Error, "EEEE") || strings.Contains(rec.Error, stderrFloodTail) {
		t.Errorf("error にコマンドの標準エラー出力が載っている: %q", rec.Error)
	}
	if !strings.Contains(err.Error(), "終了コード 3") {
		t.Errorf("返り値のエラーが終了コードを含んでいない: %v", err)
	}
}

func TestCommandRunCapsExitErrorStderr(t *testing.T) {
	// 画面に出す抜粋は末尾だけ残す。原因は最後の数行に出るためである。
	_, _, err := runStderrFlood(t, floodOverExcerptKiB)

	var exitErr *ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("*ExitError が返っていない: %v", err)
	}
	if limit := maxStderrExcerptBytes + len(limit.Prefix); len(exitErr.Stderr) > limit {
		t.Errorf("ExitError.Stderr = %d バイト, want <= %d", len(exitErr.Stderr), limit)
	}
	if !strings.HasPrefix(exitErr.Stderr, limit.Prefix) {
		t.Errorf("省略の印が付いていない: %q", exitErr.Stderr[:min(len(exitErr.Stderr), 32)])
	}
	if !strings.HasSuffix(exitErr.Stderr, stderrFloodTail) {
		t.Error("末尾ではなく先頭を残している（原因の行が落ちている）")
	}
}

func TestCommandRunCapsStderrCapture(t *testing.T) {
	// 暴走した子でメモリを食い潰さないよう、取り込み自体に上限を置く。
	// それでもプロセスは完走させる（終了コードを観測する必要があるため）。
	res, _, _ := runStderrFlood(t, floodOverCaptureKiB)

	if len(res.Stderr) != maxStderrCaptureBytes {
		t.Errorf("Result.Stderr = %d バイト, want %d", len(res.Stderr), maxStderrCaptureBytes)
	}
	if res.ExitCode != 3 {
		t.Errorf("ExitCode = %d, want 3（上限で打ち切られている）", res.ExitCode)
	}
}

func TestCommandRunKeepsShortStderrInError(t *testing.T) {
	// 上限を入れても、通常の短い標準エラー出力は丸ごと画面に出す。
	name, args := helperCommand()
	ctx := exec.WithOptions(context.Background(), exec.Options{
		Env: helperEnv(helperExitEnv+"=1", helperStderrEnv+"=だめでした: 権限がありません"),
	})

	_, err := New(NoSecrets).Run(ctx, name, args...)
	if err == nil {
		t.Fatal("非ゼロ終了でエラーを返していない")
	}
	if !strings.Contains(err.Error(), "だめでした: 権限がありません") {
		t.Errorf("短い標準エラー出力が欠けている: %v", err)
	}
	if strings.Contains(err.Error(), limit.Prefix) {
		t.Errorf("切っていないのに省略の印が付いている: %v", err)
	}
}

func TestCommandRunKeepsStderrTailOverCaptureLimit(t *testing.T) {
	// 取り込み上限を超えた場合も残すのは末尾でなければならない。取り込みが先頭を
	// 残すと limit.Head が末尾を探しても既になく、原因の行が画面から消える。
	res, _, err := runStderrFlood(t, floodOverCaptureKiB)

	if !bytes.HasSuffix(res.Stderr, []byte(stderrFloodTail)) {
		t.Error("取り込みが末尾ではなく先頭を残している")
	}

	var exitErr *ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("*ExitError が返っていない: %v", err)
	}
	if !strings.HasPrefix(exitErr.Stderr, limit.Prefix) {
		t.Errorf("省略の印が付いていない: %q", exitErr.Stderr[:min(len(exitErr.Stderr), 32)])
	}
	if !strings.HasSuffix(exitErr.Stderr, stderrFloodTail) {
		t.Error("抜粋に原因の行が残っていない（取り込み段で末尾が落ちている）")
	}
}

// floodMultiByte は 1 文字 3 バイトの埋め草。1023 バイト（341 文字）に割り切れる
// ので、1 行のバイト数は ASCII のときと変わらない。
const floodMultiByte = "あ"

func TestCommandRunKeepsExitErrorStderrValidUTF8(t *testing.T) {
	// 抜粋はバイト数で切るので、多バイト文字が流れていれば切り位置は必ず文字の
	// 途中に当たる。先頭に残った断片を落とさないと limit.Prefix の直後が壊れた
	// 文字になり、画面にも err.Error() にも置換文字が出る。
	//
	// **この経路の素材は長く ASCII だけだった。** stderrflood が吐くのは "E" の
	// 連なりで、末尾 4 KiB をどこで切っても文字の途中に当たらないため、
	// limit の dropPartialRuneAtStart が断片を落とす行は 1 度も実行されて
	// いなかった（Issue #183）。
	_, _, err := runStderrFloodFill(t, floodOverExcerptKiB, floodMultiByte)

	var exitErr *ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("*ExitError が返っていない: %v", err)
	}
	if !utf8.ValidString(exitErr.Stderr) {
		t.Error("ExitError.Stderr が壊れた UTF-8 を含んでいる")
	}
	if strings.ContainsRune(exitErr.Stderr, utf8.RuneError) {
		t.Error("ExitError.Stderr に置換文字が混じっている")
	}

	body, cut := strings.CutPrefix(exitErr.Stderr, limit.Prefix)
	if !cut {
		t.Fatalf("省略の印が付いていない: %q", exitErr.Stderr[:min(len(exitErr.Stderr), 32)])
	}
	// 印の直後が文字の途中で始まっていないこと。ここが要点である。
	if r, size := utf8.DecodeRuneInString(body); r == utf8.RuneError && size <= 1 {
		t.Error("省略の印の直後が壊れた文字で始まっている")
	}

	// **素材が経路を通していることを確かめる（陽性対照）。** 断片を落としたぶんだけ
	// 抜粋は上限より短くなる。ちょうど上限なら切り位置が文字の境界に当たっており、
	// 断片を落とす行が実行されていない——そのときは上の 3 つが壊れた実装でも
	// 緑になるので、素材か量を見直すこと。
	if len(body) >= maxStderrExcerptBytes {
		t.Fatalf("抜粋が %d バイトあり、切り位置が文字の境界に当たっている（素材が経路を通していない）", len(body))
	}
	if !strings.HasSuffix(exitErr.Stderr, stderrFloodTail) {
		t.Error("末尾ではなく先頭を残している（原因の行が落ちている）")
	}
}
