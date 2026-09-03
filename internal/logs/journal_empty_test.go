package logs

import (
	"context"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/ousiassllc/gsr-helper/internal/exec"
)

// 空の取得（終了コード 0・出力なし）を挟んだときの追従を確かめる。
//
// journal_test.go と分けたのは、そちらが 240 行を超えていて 1 ファイル 300 行の
// 上限に近いためである。reopen_test.go を tail_test.go から分けたのと同じ理由で、
// 質的にまとまった 1 件を別ファイルへ置く。

// journalEmptyDeadline は取得の回数が目当ての回数に達するのを待つ上限。
//
// 取得は JournalInterval(2 秒) ごとなので 4 回目は 6 秒前後で始まる。不具合で
// 追従が止まった場合に無限待ちにしないためだけの値であり、余裕を持たせてある。
const journalEmptyDeadline = 30 * time.Second

// 空の取得を挟んでも、同じ行が二重に届かない（FR-26 の追従）。
//
// `journalctl` はユニットの再起動やジャーナルの回転と重なると、成功したまま
// 1 行も返さないことがある。これを「ログが空になった」と受け取って覚えている
// 末尾を捨てる実装だと、次の取得で重なりが求まらず、変わっていない末尾が
// 丸ごと送り直される。修正前はこの手順で out に a b c a b c が届く。
//
// **判定は「4 回目の取得が始まった」時点で行う。** 送り直しが起きるのは空の
// 次（3 回目）の取得なので、そこまで進めないと見逃す。取得は 1 本の goroutine が
// 「取得 → 送出 → 待ち」を繰り返す形なので、4 回目に入った時点で 3 回目の送出は
// すべて終わっている。時間で待つ代わりにこの順序に乗せることで、負荷で揺れても
// 判定がぶれない。
func TestJournalKeepsPrevOnEmptyFetch(t *testing.T) {
	var mu sync.Mutex
	calls := 0
	fourth := make(chan struct{})
	fake := exec.NewFake()
	fake.SetFunc(func(string, []string) (exec.Result, error) {
		mu.Lock()
		defer mu.Unlock()
		calls++
		switch calls {
		case 2:
			// 成功しているのに 1 行も返らない取得。
			return exec.Result{Stdout: nil, Stderr: nil, ExitCode: 0}, nil
		case 4:
			close(fourth)
		}
		return exec.Result{Stdout: []byte("a\nb\nc\n"), Stderr: nil, ExitCode: 0}, nil
	})

	ctx, cancel := context.WithCancel(t.Context())
	ch := make(chan Line, 64)
	done := make(chan error, 1)
	go func() { done <- Journal(ctx, fake, "u.service", ch) }()

	select {
	case <-fourth:
	case <-time.After(journalEmptyDeadline):
		cancel()
		t.Fatal("4 回目の取得まで進まない（追従が止まっている）")
	}
	cancel()
	if err := <-done; err != nil {
		t.Errorf("Journal が失敗した: %v", err)
	}

	var got []string
	for l := range ch {
		got = append(got, l.Text)
	}
	if want := []string{"a", "b", "c"}; !slices.Equal(got, want) {
		t.Errorf("届いた行 = %v, want %v（空の取得で末尾を忘れ、同じ行を送り直している）", got, want)
	}
}
