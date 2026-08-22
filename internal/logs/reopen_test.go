package logs

import (
	"os"
	"testing"
	"time"
)

// ログの入れ替え（同じ名前・別 inode）への追従を確かめる。
//
// tail_test.go と分けたのは、フィクスチャの作り方（unlink してから作り直す）が
// 追記や切り詰めのテストと質的に違うためである。1 ファイルの行数上限にも近い。

// 同じ名前で作り直されたら開き直し、その後の追記も届く。
//
// 開き直さない実装では、unlink 済みの古い inode を読み続ける。古い inode には
// 誰も書かないので追記は永久に届かず、サイズも変わらないため切り詰め検知も
// 働かない。つまりこのテストは修正前にはタイムアウトで落ちる。
func TestTailFollowsRecreatedFile(t *testing.T) {
	path := writeLog(t, t.TempDir(), "Runner_1.log", "old\n", time.Now())
	ch, _ := startTail(t, path)
	if got := next(t, ch); got.Text != "old" {
		t.Fatalf("1 行目 = %q, want old", got.Text)
	}

	// unlink してから作り直す。os.WriteFile による上書き（切り詰め）と違い、
	// inode が変わるので、開いたままのハンドルでは追えなくなる。
	if err := os.Remove(path); err != nil {
		t.Fatalf("ログを消せない: %v", err)
	}
	if err := os.WriteFile(path, []byte("recreated\n"), 0o600); err != nil {
		t.Fatalf("ログを作り直せない: %v", err)
	}
	appendLine(t, path, "after recreate\n")

	// 作成の通知と中身の書き込みは前後しうるため、届く行数は 1 行にも 2 行にも
	// なる。順序だけを見て、最後の追記に辿り着けることを確かめる。
	sawRecreated := false
	for {
		got := next(t, ch)
		if got.Text == "after recreate" {
			break
		}
		if got.Text != "recreated" {
			t.Fatalf("作り直し後の行 = %q, want recreated か after recreate", got.Text)
		}
		sawRecreated = true
	}
	if !sawRecreated {
		t.Error("作り直した直後の行（recreated）が届いていない")
	}
}
