package hostres_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/doctor/check"
	"github.com/ousiassllc/gsr-helper/internal/exec"
)

// syncStatus は timesync-status の実出力を模した本文を組み立てる。
//
// Offset 以外の行を残すのは、パーサが見出しの前方一致だけで拾っていることを
// 確かめるためである（"Poll interval: 34min 8s" のように時間表記を含む行が
// 前後に並ぶ）。
func syncStatus(offsetLine string) string {
	return "       Server: 10.0.0.1 (ntp.example.com)\n" +
		"Poll interval: 34min 8s (min: 32s; max 34min 8s)\n" +
		"Root distance: 4.027ms (max: 5s)\n" +
		offsetLine +
		"        Delay: 234.350ms\n" +
		"       Jitter: 6.360ms\n"
}

// timedatectlFake は timedatectl の 2 つの呼び出しを引数で振り分ける。
//
// ntpCheck は同期状態を `show` から、ずれを `timesync-status` から取るので、
// 単一の出力を返すだけの Fake では両者を作り分けられない。
func timedatectlFake(show string, sync exec.Result) *exec.Fake {
	return fakeExec(func(_ string, args []string) (exec.Result, error) {
		if len(args) > 0 && args[0] == "timesync-status" {
			return sync, nil
		}
		return okResult(show), nil
	})
}

const (
	showSynced   = "NTP=yes\nNTPSynchronized=yes\nTimeUSec=Sat 2026-08-23 12:00:00 JST\n"
	showUnsynced = "NTP=yes\nNTPSynchronized=no\nTimeUSec=Sat 2026-08-23 12:00:00 JST\n"
)

func TestNTP(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		stdout string
		look   func(string) (string, error)
		want   check.Status
	}{
		"同期済み": {
			stdout: showSynced,
			look:   lookOnly("timedatectl"), want: check.OK,
		},
		"未同期": {
			stdout: showUnsynced,
			look:   lookOnly("timedatectl"), want: check.Fail,
		},
		"NTP そのものが無効": {
			stdout: "NTP=no\nNTPSynchronized=no\nTimeUSec=Sat 2026-08-23 12:00:00 JST\n",
			look:   lookOnly("timedatectl"), want: check.Fail,
		},
		"timedatectl が無い": {
			stdout: "", look: lookOnly(), want: check.Skip,
		},
		"出力に NTPSynchronized が無い": {
			stdout: "TimeUSec=Sat 2026-08-23 12:00:00 JST\n",
			look:   lookOnly("timedatectl"), want: check.Skip,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			in := check.Input{
				Exec:     timedatectlFake(tt.stdout, okResult("")),
				LookPath: tt.look,
			}
			got := only(t, run(t, "time.ntp", in))
			if got.Status != tt.want {
				t.Errorf("Status = %v, want %v（Detail: %s）", got.Status, tt.want, got.Detail)
			}
		})
	}
}

// ずれは timesync-status のオフセットから出す。正・負・0 と、取れない場合の
// 縮退（同期状態だけを出し、理由を Detail に書く）を固定する。
func TestNTPOffset(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		show       string
		sync       exec.Result
		wantSum    string
		wantDetail string
	}{
		"未同期・正のオフセット": {
			show: showUnsynced, sync: okResult(syncStatus("       Offset: +42s\n")),
			wantSum: "NTP 未同期（ずれ 42 秒）", wantDetail: "NTP サーバより 42 秒 遅れています",
		},
		"未同期・負のオフセット": {
			show: showUnsynced, sync: okResult(syncStatus("       Offset: -1min 30s\n")),
			wantSum: "NTP 未同期（ずれ 90 秒）", wantDetail: "NTP サーバより 90 秒 進んでいます",
		},
		"未同期・オフセット 0": {
			show: showUnsynced, sync: okResult(syncStatus("       Offset: +0\n")),
			wantSum: "NTP 未同期（ずれ 0 ミリ秒）", wantDetail: "ずれは検出されませんでした",
		},
		"未同期・1 秒未満はミリ秒": {
			show: showUnsynced, sync: okResult(syncStatus("       Offset: +2.352ms\n")),
			wantSum: "NTP 未同期（ずれ 2 ミリ秒）", wantDetail: "NTP サーバより 2 ミリ秒 遅れています",
		},
		"未同期・timesync-status が失敗": {
			show: showUnsynced, sync: exec.Result{Stdout: nil, Stderr: []byte("Failed to query server"), ExitCode: 1},
			wantSum: "NTP 未同期", wantDetail: "ずれの秒数は取得できませんでした",
		},
		"未同期・Offset 行が無い": {
			show: showUnsynced, sync: okResult(syncStatus("")),
			wantSum: "NTP 未同期", wantDetail: "chronyd",
		},
		"未同期・解釈できない書式": {
			show: showUnsynced, sync: okResult(syncStatus("       Offset: (ignored)\n")),
			wantSum: "NTP 未同期", wantDetail: "ずれの秒数は取得できませんでした",
		},
		"同期済みでもオフセットが取れれば出す": {
			show: showSynced, sync: okResult(syncStatus("       Offset: +2.352ms\n")),
			wantSum: "NTP 同期済み", wantDetail: "NTP サーバより 2 ミリ秒 遅れています",
		},
		"同期済み・オフセットが取れない": {
			show: showSynced, sync: exec.Result{Stdout: nil, Stderr: nil, ExitCode: 1},
			wantSum: "NTP 同期済み", wantDetail: "ずれの秒数は取得できませんでした",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			in := check.Input{
				Exec:     timedatectlFake(tt.show, tt.sync),
				LookPath: lookOnly("timedatectl"),
			}
			got := only(t, run(t, "time.ntp", in))
			if got.Summary != tt.wantSum {
				t.Errorf("Summary = %q, want %q", got.Summary, tt.wantSum)
			}
			if !strings.Contains(got.Detail, tt.wantDetail) {
				t.Errorf("Detail に %q が無い: %s", tt.wantDetail, got.Detail)
			}
		})
	}
}

// オフセットを取れないことは判定不能ではない。同期状態は timedatectl show だけで
// 決まるので、SKIP へ倒さず OK / FAIL を出し切ること。
func TestNTPKeepsVerdictWithoutOffset(t *testing.T) {
	t.Parallel()

	in := check.Input{
		Exec: timedatectlFake(showUnsynced,
			exec.Result{Stdout: nil, Stderr: []byte("Failed to query server: Unit dbus-org... not found"), ExitCode: 1}),
		LookPath: lookOnly("timedatectl"),
	}
	got := only(t, run(t, "time.ntp", in))
	if got.Status != check.Fail {
		t.Errorf("Status = %v, want %v（ずれが取れなくても未同期は未同期）", got.Status, check.Fail)
	}
	if !strings.Contains(got.Remedy, "timedatectl set-ntp true") {
		t.Errorf("Remedy に同期を有効にする手順が無い: %s", got.Remedy)
	}
}

// ずれは外向きの通信ではなく timesyncd が算出済みの値から取る。引数を固定して
// 「診断が NTP サーバへ問い合わせに行く形」へ戻らないようにする。
func TestNTPReadsOffsetLocally(t *testing.T) {
	t.Parallel()

	f := timedatectlFake(showUnsynced, okResult(syncStatus("       Offset: +42s\n")))
	in := check.Input{Exec: f, LookPath: lookOnly("timedatectl")}
	if got := only(t, run(t, "time.ntp", in)).Status; got != check.Fail {
		t.Fatalf("Status = %v, want %v", got, check.Fail)
	}

	calls := f.Calls()
	if len(calls) != 2 {
		t.Fatalf("呼び出し回数 = %d, want 2（show と timesync-status）", len(calls))
	}
	want := []string{"timesync-status", "--no-pager"}
	if !slices.Equal(calls[1].Args, want) {
		t.Errorf("引数 = %q, want %q", calls[1].Args, want)
	}
	if calls[1].Name != "timedatectl" {
		t.Errorf("コマンド = %q, want %q", calls[1].Name, "timedatectl")
	}
}
