package disk

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// WorkUsage は _work 全体の合計を返す（Issue #73）。
//
// 内訳（Scan）とは別に、Runners タブの `_WORK` 列へ出す 1 台 1 値を得るための経路。

// write は dir/name に size バイトのファイルを作る。
func write(t *testing.T, dir, name string, size int) {
	t.Helper()

	full := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(full, make([]byte, size), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

func TestWorkUsageSumsWholeWorkTree(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "_work/myrepo/myrepo/a.bin", 100)
	write(t, dir, "_work/_tool/node/b.bin", 20)
	write(t, dir, "_work/_temp/c.bin", 3)
	// _work の外は数えない（_diag は削除の単位としては別枠。scanTargets を参照）。
	write(t, dir, "_diag/Worker_x.log", 9999)

	got, err := WorkUsage(context.Background(), newRunner(dir, false))
	if err != nil {
		t.Fatalf("WorkUsage: %v", err)
	}
	if want := int64(123); got != want {
		t.Errorf("WorkUsage = %d, want %d（_work 配下だけの合計）", got, want)
	}
}

// _work を持たない runner は 0 とエラー無しになる。
//
// ジョブを 1 度も実行していない runner は _work を持たない。異常として返すと、
// 起動直後の一覧が「集計失敗」で埋まる。
func TestWorkUsageWithoutWorkDirIsZero(t *testing.T) {
	got, err := WorkUsage(context.Background(), newRunner(t.TempDir(), false))
	if err != nil {
		t.Fatalf("WorkUsage がエラーを返した: %v", err)
	}
	if got != 0 {
		t.Errorf("WorkUsage = %d, want 0", got)
	}
}

// 打ち切られたら途中までの値を返さない。
//
// 途中経過を確定値として見せると、実際より小さい使用量を読ませる。
func TestWorkUsageCanceledReturnsError(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "_work/myrepo/a.bin", 10)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	got, err := WorkUsage(ctx, newRunner(dir, false))
	if err == nil {
		t.Fatal("打ち切られたのにエラーを返していない")
	}
	if got != 0 {
		t.Errorf("WorkUsage = %d, want 0（途中経過を返してはいけない）", got)
	}
}
