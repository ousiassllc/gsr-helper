package jobreq_test

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ousiassllc/gsr-helper/internal/doctor/check"
	"github.com/ousiassllc/gsr-helper/internal/exec"
	"github.com/ousiassllc/gsr-helper/internal/runner"
)

// `sudo -l -U` はユーザー名しか受け付けない。UID を渡す場合は #1001 の形式が要る。
//
// 付けずに渡すと「そんなユーザーは居ない」という失敗になり、NOPASSWD が無いのと
// 区別できない（security.md「パスワード不要 sudo の要求への対応」）。
//
// 併せて `-n`（非対話）が必ず先頭に付くことも見る。無いと、パスワードを要求する
// ホストで sudo が /dev/tty へプロンプトを書き込み、起動時の自動判定（FR-44）で
// TUI の alternate screen が壊れたまま runner ごとに入力待ちになる。
func TestSudoPassesUIDWithHashPrefix(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		user string
		want string
	}{
		"ユーザー名はそのまま渡す":     {user: "runner", want: "runner"},
		"UID は # を付けて渡す":   {user: "1001", want: "#1001"},
		"数字で始まる名前は名前として渡す": {user: "1001runner", want: "1001runner"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			f := fakeExec(func(string, []string) (exec.Result, error) {
				return okResult("(ALL) NOPASSWD: ALL"), nil
			})
			in := check.Input{
				Runners:  []runner.Runner{newRunner("build01", tt.user, 0)},
				Exec:     f,
				LookPath: lookOnly("sudo"),
			}

			if got := only(t, run(t, "job.sudo", in)).Status; got != check.OK {
				t.Errorf("Status = %v, want %v", got, check.OK)
			}

			calls := f.Calls()
			if len(calls) != 1 {
				t.Fatalf("呼び出し回数 = %d, want 1", len(calls))
			}
			want := []string{"-n", "-l", "-U", tt.want}
			if !slices.Equal(calls[0].Args, want) {
				t.Errorf("引数 = %q, want %q", calls[0].Args, want)
			}
		})
	}
}

// NOPASSWD の欠落は WARN であり FAIL にしない。付与は実質 root を与えることを
// 意味するため、可否は運用者に委ねる（FR-43）。
func TestSudoMissingNopasswdIsWarnNotFail(t *testing.T) {
	t.Parallel()

	f := fakeExec(func(string, []string) (exec.Result, error) {
		return okResult("(ALL) ALL"), nil
	})
	in := check.Input{
		Runners:  []runner.Runner{newRunner("build01", "runner", 0)},
		Exec:     f,
		LookPath: lookOnly("sudo"),
	}

	got := only(t, run(t, "job.sudo", in))
	if got.Status != check.Warn {
		t.Fatalf("Status = %v, want %v", got.Status, check.Warn)
	}
	if !strings.Contains(got.Remedy, "visudo -c") {
		t.Errorf("Remedy に visudo -c の検証が無い（sudoers を壊すと復旧できない）: %s", got.Remedy)
	}
}

// Remedy に出す sudoers 行も UID には # が要る。
//
// sudoers はユーザー名の欄に UID を書く場合 `#1001` の形式しか解さない。生の UID を
// 埋めた行は貼っても効かず、`visudo -c` は通ってしまうので運用者が気付けない。
func TestSudoRemedyUsesHashPrefixedUID(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		user string
		want string
	}{
		"UID は # を付けた行を出す": {user: "1001", want: "#1001 ALL=(ALL) NOPASSWD: ALL"},
		"ユーザー名はそのまま出す":     {user: "runner", want: "runner ALL=(ALL) NOPASSWD: ALL"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			in := check.Input{
				Runners: []runner.Runner{newRunner("build01", tt.user, 0)},
				Exec: fakeExec(func(string, []string) (exec.Result, error) {
					return okResult("(ALL) ALL"), nil
				}),
				LookPath: lookOnly("sudo"),
			}

			got := only(t, run(t, "job.sudo", in))
			if got.Status != check.Warn {
				t.Fatalf("Status = %v, want %v", got.Status, check.Warn)
			}
			if !strings.Contains(got.Remedy, tt.want) {
				t.Errorf("Remedy に %q が無い（貼っても効かない sudoers 行になっている）: %s", tt.want, got.Remedy)
			}
		})
	}
}

// 期限切れ・キャンセルは WARN ではなく SKIP。
//
// 起動時の判定には全体で 10 秒の上限があり（internal/ui/hostreq）、遅いホストでは
// そこで打ち切られる。WARN にすると起動直後の要注意件数が偽の警告で膨らみ、
// 「判定を諦める。警告が出ないだけで起動は妨げない」という hostreq の定めと矛盾する。
func TestSudoExpiredContextIsSkippedNotWarned(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		ctx func(t *testing.T) context.Context
	}{
		"キャンセル済み": {ctx: func(t *testing.T) context.Context {
			t.Helper()
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			return ctx
		}},
		"期限切れ": {ctx: func(t *testing.T) context.Context {
			t.Helper()
			ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
			t.Cleanup(cancel)
			return ctx
		}},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			in := check.Input{
				Runners: []runner.Runner{newRunner("build01", "runner", 0)},
				Exec: fakeExec(func(string, []string) (exec.Result, error) {
					return okResult("(ALL) NOPASSWD: ALL"), nil
				}),
				LookPath: lookOnly("sudo"),
			}

			got := only(t, checkByID(t, "job.sudo").Run(tt.ctx(t), in))
			if got.Status != check.Skip {
				t.Errorf("Status = %v, want %v（Detail: %s）", got.Status, check.Skip, got.Detail)
			}
			if got.Remedy != "" {
				t.Errorf("SKIP なのに対処がある: %q", got.Remedy)
			}
		})
	}
}

// 権限の一覧そのものは画面に写さない。判定の結果だけを出す。
func TestSudoDoesNotEchoCommandOutput(t *testing.T) {
	t.Parallel()

	const secret = "(root) NOPASSWD: /usr/bin/internal-deploy-secret"
	f := fakeExec(func(string, []string) (exec.Result, error) {
		return okResult(secret), nil
	})
	in := check.Input{
		Runners:  []runner.Runner{newRunner("build01", "runner", 0)},
		Exec:     f,
		LookPath: lookOnly("sudo"),
	}

	got := only(t, run(t, "job.sudo", in))
	for _, field := range []string{got.Detail, got.Summary, got.Impact, got.Remedy} {
		if strings.Contains(field, "internal-deploy-secret") {
			t.Errorf("sudo の出力が画面の文言へ写っている: %s", field)
		}
	}
}

// sudo が無ければ FAIL ではなく SKIP。
func TestSudoWithoutCommandIsSkipped(t *testing.T) {
	t.Parallel()

	in := check.Input{
		Runners:  []runner.Runner{newRunner("build01", "runner", 0)},
		Exec:     exec.NewFake(),
		LookPath: lookOnly(),
	}
	if got := only(t, run(t, "job.sudo", in)).Status; got != check.Skip {
		t.Errorf("Status = %v, want %v", got, check.Skip)
	}
}
