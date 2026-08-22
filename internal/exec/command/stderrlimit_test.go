package command

import (
	"bytes"
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/exec"
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

// runStderrFlood は kib KiB の標準エラー出力を吐いて非ゼロで終わるコマンドを
// 実行し、監査ログの内容とともに返す。
func runStderrFlood(t *testing.T, kib int) (exec.Result, *bytes.Buffer, error) {
	t.Helper()

	var buf bytes.Buffer
	c := New(NoSecrets, WithAudit(testLogger(&buf)))

	name, args := helperCommand()
	ctx := exec.WithOptions(context.Background(), exec.Options{
		Action: "test.flood",
		Env:    helperEnv(helperModeEnv+"=stderrflood", helperExitEnv+"=3", helperStderrKiB+"="+strconv.Itoa(kib)),
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
	if limit := maxStderrExcerptBytes + len(elisionPrefix); len(exitErr.Stderr) > limit {
		t.Errorf("ExitError.Stderr = %d バイト, want <= %d", len(exitErr.Stderr), limit)
	}
	if !strings.HasPrefix(exitErr.Stderr, elisionPrefix) {
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
	if strings.Contains(err.Error(), elisionPrefix) {
		t.Errorf("切っていないのに省略の印が付いている: %v", err)
	}
}

func TestCommandRunKeepsStderrTailOverCaptureLimit(t *testing.T) {
	// 取り込み上限を超えた場合も残すのは末尾でなければならない。取り込みが先頭を
	// 残すと truncateHead が末尾を探しても既になく、原因の行が画面から消える。
	res, _, err := runStderrFlood(t, floodOverCaptureKiB)

	if !bytes.HasSuffix(res.Stderr, []byte(stderrFloodTail)) {
		t.Error("取り込みが末尾ではなく先頭を残している")
	}

	var exitErr *ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("*ExitError が返っていない: %v", err)
	}
	if !strings.HasPrefix(exitErr.Stderr, elisionPrefix) {
		t.Errorf("省略の印が付いていない: %q", exitErr.Stderr[:min(len(exitErr.Stderr), 32)])
	}
	if !strings.HasSuffix(exitErr.Stderr, stderrFloodTail) {
		t.Error("抜粋に原因の行が残っていない（取り込み段で末尾が落ちている）")
	}
}

func TestLimitedBufferRetainsLastBytes(t *testing.T) {
	// 小さな書き込みの積み上げでも 1 回の巨大な書き込みでも、残るのは末尾の
	// limit バイトだけで、保持量が limit を超えない。
	b := &limitedBuffer{limit: 8}
	for i := range 10 {
		if _, err := b.Write([]byte{byte('0' + i)}); err != nil {
			t.Fatalf("Write が失敗した: %v", err)
		}
	}
	if got := b.String(); got != "23456789" {
		t.Errorf("小さな書き込みの積み上げ = %q, want %q", got, "23456789")
	}

	if n, err := b.Write([]byte("ABCDEFGHIJKL")); n != 12 || err != nil {
		t.Fatalf("Write = (%d, %v), want (12, nil)", n, err)
	}
	if got := b.String(); got != "EFGHIJKL" {
		t.Errorf("上限を 1 回で埋める書き込み = %q, want %q", got, "EFGHIJKL")
	}

	zero := &limitedBuffer{}
	if n, err := zero.Write([]byte("x")); n != 1 || err != nil || len(zero.Bytes()) != 0 {
		t.Errorf("limit 0 で (%d, %v, %d バイト保持), want (1, nil, 0 バイト)", n, err, len(zero.Bytes()))
	}
}
