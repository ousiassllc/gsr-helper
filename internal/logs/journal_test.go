package logs

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ousiassllc/gsr-helper/internal/exec"
)

// collect は Journal を動かし、want 行を受け取った時点で止めて全行を返す。
func collect(t *testing.T, ex exec.Executor, unit string, want int) []Line {
	t.Helper()

	ctx, cancel := context.WithCancel(t.Context())
	ch := make(chan Line, 64)
	done := make(chan error, 1)
	go func() { done <- Journal(ctx, ex, unit, ch) }()

	var got []Line
	deadline := time.After(10 * time.Second)
	for len(got) < want {
		select {
		case l, ok := <-ch:
			if !ok {
				t.Fatalf("行が %d 件しか届かないままチャネルが閉じた", len(got))
			}
			got = append(got, l)
		case <-deadline:
			cancel()
			t.Fatalf("行が届かない（%d/%d）", len(got), want)
		}
	}
	cancel()
	if err := <-done; err != nil {
		t.Errorf("Journal が失敗した: %v", err)
	}
	return got
}

// 発行するコマンドは journalctl -u <unit> -n <N> --no-pager である
// （docs/api/external-interfaces.md）。exec のテスト実装で検証する。
func TestJournalRunsExpectedCommand(t *testing.T) {
	fake := exec.NewFake()
	fake.SetFunc(func(string, []string) (exec.Result, error) {
		return exec.Result{Stdout: []byte("line 1\n"), Stderr: nil, ExitCode: 0}, nil
	})

	collect(t, fake, "actions.runner.foo.build01-1.service", 1)

	calls := fake.Calls()
	if len(calls) == 0 {
		t.Fatal("journalctl を 1 度も実行していない")
	}
	got := calls[0]
	want := "journalctl -u actions.runner.foo.build01-1.service -n 200 --no-pager"
	if got.String() != want {
		t.Errorf("発行コマンド = %q, want %q", got.String(), want)
	}
	if !got.HasDeadline {
		t.Error("タイムアウトが課されていない")
	}
}

// 追従の取得は監査ログに記録しない（繰り返し発行され他のレコードを押し流すため）。
func TestJournalSkipsAudit(t *testing.T) {
	fake := exec.NewFake()
	fake.SetFunc(func(string, []string) (exec.Result, error) {
		return exec.Result{Stdout: []byte("a\n"), Stderr: nil, ExitCode: 0}, nil
	})

	collect(t, fake, "u.service", 1)

	got := fake.Calls()[0].Options
	if !got.SkipAudit {
		t.Error("SkipAudit が偽（追従の取得が監査ログへ毎回書かれる）")
	}
	if got.Action != journalAction {
		t.Errorf("action = %q, want %q", got.Action, journalAction)
	}
}

// 出力は重大度付きの行として送る（FR-25 の強調表示に使う）。
func TestJournalSendsClassifiedLines(t *testing.T) {
	fake := exec.NewFake()
	fake.SetFunc(func(string, []string) (exec.Result, error) {
		return exec.Result{Stdout: []byte("info\n[ERROR] boom\n"), Stderr: nil, ExitCode: 0}, nil
	})

	got := collect(t, fake, "u.service", 2)
	if got[0].Text != "info" || got[0].Level != LevelPlain {
		t.Errorf("1 行目 = %+v, want {info LevelPlain}", got[0])
	}
	if got[1].Text != "[ERROR] boom" || got[1].Level != LevelError {
		t.Errorf("2 行目 = %+v, want {[ERROR] boom LevelError}", got[1])
	}
}

// 取得のたびに返る同じ末尾は送り直さず、増えた分だけを送る（FR-26 の追従）。
func TestJournalSendsOnlyNewLines(t *testing.T) {
	var mu sync.Mutex
	n := 0
	fake := exec.NewFake()
	fake.SetFunc(func(string, []string) (exec.Result, error) {
		mu.Lock()
		defer mu.Unlock()
		n++
		if n == 1 {
			return exec.Result{Stdout: []byte("a\nb\n"), Stderr: nil, ExitCode: 0}, nil
		}
		return exec.Result{Stdout: []byte("a\nb\nc\n"), Stderr: nil, ExitCode: 0}, nil
	})

	got := collect(t, fake, "u.service", 3)
	want := []string{"a", "b", "c"}
	for i, w := range want {
		if got[i].Text != w {
			t.Errorf("%d 行目 = %q, want %q", i, got[i].Text, w)
		}
	}
}

// systemd ユニットを持たない runner では追従を始めない。
func TestJournalWithoutUnit(t *testing.T) {
	ch := make(chan Line, 1)
	err := Journal(t.Context(), exec.NewFake(), "", ch)
	if !errors.Is(err, ErrNoUnit) {
		t.Errorf("err = %v, want ErrNoUnit", err)
	}
	if _, ok := <-ch; ok {
		t.Error("チャネルが閉じられていない")
	}
}

// 外部コマンドを実行できない（Executor が無い）なら、その旨を返して始めない。
//
// 依存の組み立てを間違えた場合に nil のまま呼ばれると、追従が始まらない理由が
// どこにも出ない。ここで理由を返すことを固定しておく。
func TestJournalWithoutExecutor(t *testing.T) {
	ch := make(chan Line, 1)
	err := Journal(t.Context(), nil, "u.service", ch)
	if err == nil {
		t.Fatal("Executor が nil でエラーにならなかった")
	}
	if _, ok := <-ch; ok {
		t.Error("チャネルが閉じられていない")
	}
}

// journalctl が異常終了し続けたら理由を返す（空表示にして原因を隠さない）。
func TestJournalNonZeroExit(t *testing.T) {
	fake := exec.NewFake()
	fake.SetFunc(func(string, []string) (exec.Result, error) {
		return exec.Result{Stdout: nil, Stderr: []byte("no such unit"), ExitCode: 1}, nil
	})

	ch := make(chan Line, 1)
	err := Journal(t.Context(), fake, "u.service", ch)
	if err == nil {
		t.Fatal("異常終了でエラーにならなかった")
	}
	if !strings.Contains(err.Error(), "u.service") {
		t.Errorf("エラー文 = %q, want ユニット名を含む", err.Error())
	}
}

// 実行そのものに失敗し続けたら、上限回数まで試してからエラーを返して終わる。
//
// 終了コードではなく Run 自体がエラーを返す経路（journalctl が無い、権限が
// 足りない）を通す。ここが素通りすると、回復しない失敗でも粘り続けて
// 空の画面を見せ続けることになる。
func TestJournalRunError(t *testing.T) {
	fake := exec.NewFake()
	fake.SetFunc(func(string, []string) (exec.Result, error) {
		return exec.Result{Stdout: nil, Stderr: nil, ExitCode: -1}, errors.New("executable file not found")
	})

	ch := make(chan Line, 1)
	err := Journal(t.Context(), fake, "u.service", ch)
	if err == nil {
		t.Fatal("実行に失敗してもエラーにならなかった")
	}
	if !strings.Contains(err.Error(), "u.service") {
		t.Errorf("エラー文 = %q, want ユニット名を含む", err.Error())
	}
	if got := len(fake.Calls()); got != journalRetries {
		t.Errorf("試行回数 = %d, want %d（上限まで再試行していない）", got, journalRetries)
	}
	if _, ok := <-ch; ok {
		t.Error("チャネルが閉じられていない")
	}
}

// 一時的な取得失敗は挟んでも追従が続く（利用者に押し直させない）。
//
// 上限に 1 回だけ足りない回数を失敗させてから成功させる。失敗を数え直して
// いなければ、この後さらに失敗したときに即座に終わってしまう。
func TestJournalRetriesTransientFailure(t *testing.T) {
	var mu sync.Mutex
	n := 0
	fake := exec.NewFake()
	fake.SetFunc(func(string, []string) (exec.Result, error) {
		mu.Lock()
		defer mu.Unlock()
		n++
		if n < journalRetries {
			return exec.Result{Stdout: nil, Stderr: nil, ExitCode: -1}, errors.New("一時的な失敗")
		}
		return exec.Result{Stdout: []byte("recovered\n"), Stderr: nil, ExitCode: 0}, nil
	})

	got := collect(t, fake, "u.service", 1)
	if got[0].Text != "recovered" {
		t.Errorf("行 = %q, want recovered", got[0].Text)
	}
}

// 重なりの判定は、前回の末尾と今回の先頭が一致する最大の長さを返す。
func TestOverlap(t *testing.T) {
	cases := map[string]struct {
		prev, cur []string
		want      int
	}{
		"初回は 0":   {nil, []string{"a", "b"}, 0},
		"全部同じ":    {[]string{"a", "b"}, []string{"a", "b"}, 2},
		"1 行増えた":  {[]string{"a", "b"}, []string{"b", "c"}, 1},
		"総入れ替え":   {[]string{"a", "b"}, []string{"c", "d"}, 0},
		"今回が短い":   {[]string{"a", "b", "c"}, []string{"c"}, 1},
		"部分一致は無視": {[]string{"ab"}, []string{"abc"}, 0},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			if got := overlap(c.prev, c.cur); got != c.want {
				t.Errorf("overlap(%v, %v) = %d, want %d", c.prev, c.cur, got, c.want)
			}
		})
	}
}
