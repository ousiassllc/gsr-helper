package logs

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

// reopenLineLen は作り直し用フィクスチャの 1 行のバイト数（改行込み）。
//
// 期待する行数はこの値と TailInitialBytes から計算する。マジックナンバーを
// 書くと、遡る量を変えたときにテストだけが取り残されて意味を失う。
const reopenLineLen = 1024

// largeLog は 1 行 reopenLineLen バイトの行を n 行並べた本文を返す。
//
// 行頭に通し番号を入れるのは、末尾の行が届いたことを内容で判定するためである。
// 全行が同じ文言だと、どこまで読んだのかを届いた行から言えない。
func largeLog(n int) string {
	var b strings.Builder
	for i := range n {
		// 6 桁の番号 + 空白 + 埋め草 + 改行 でちょうど reopenLineLen バイト。
		fmt.Fprintf(&b, "%06d %s\n", i, strings.Repeat("x", reopenLineLen-8))
	}
	return b.String()
}

// 作り直されたログも末尾だけを読む（TailInitialBytes は再オープン経路にも効く）。
//
// 初回の seekTail を通らない経路なので、開き直しで位置を先頭へ戻す実装だと
// ここだけ上限が抜ける。ローテートで退避していた大きいログを書き戻したような
// ときに全行がチャネルへ流れ、UI が受け切るまで固まる。修正前はフィクスチャの
// 全行が届いて落ちる。
//
// **差し替えは rename で行う。** os.WriteFile で直接作ると、作成の通知は中身が
// 揃う前に届きうる。開き直した時点でまだ 0 バイトだと seekTail が先頭を返し、
// 「大きいファイルを開き直した」経路を通らないままテストが緑になってしまう。
// 書き終えたものを atomically 差し替えれば、通知の時点で必ず大きい。
func TestTailReadsOnlyTailOfRecreatedFile(t *testing.T) {
	path := writeLog(t, t.TempDir(), "Runner_1.log", "old\n", time.Now())
	ch, _ := startTail(t, path)
	if got := next(t, ch); got.Text != "old" {
		t.Fatalf("1 行目 = %q, want old", got.Text)
	}

	// 遡る量の 3 倍。上限が効いていれば、届く行数は全体の 1/3 に満たない。
	total := 3 * TailInitialBytes / reopenLineLen
	tmp := filepath.Join(filepath.Dir(path), "Runner_1.log.new")
	if err := os.WriteFile(tmp, []byte(largeLog(total)), 0o600); err != nil {
		t.Fatalf("作り直すログを書けない: %v", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		t.Fatalf("ログを差し替えられない: %v", err)
	}

	// 末尾の行に辿り着くまで数える。遡った先が行の途中になると 1 行捨てるので、
	// 上限ぴったりではなく 1 行の余裕を見る。
	last := fmt.Sprintf("%06d", total-1)
	got := 0
	for {
		got++
		if strings.HasPrefix(next(t, ch).Text, last) {
			break
		}
	}
	if want := TailInitialBytes/reopenLineLen + 1; got > want {
		t.Errorf("作り直し後に届いた行数 = %d, want %d 以下（全 %d 行を先頭から流している）", got, want, total)
	}
}
