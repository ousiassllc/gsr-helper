package hostcaps

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/ousiassllc/gsr-helper/internal/appconfig/confpath"
	"github.com/ousiassllc/gsr-helper/internal/exec"
)

const testTimeout = 5 * time.Second

// testProbes はホストの状態に依存しない判定手段を組む。
// CI は systemd も docker もある self-hosted runner なので、実物を見てはいけない。
func testProbes(avail []string, env map[string]string, euid int) probes {
	return probes{
		lookPath: func(name string) (string, error) {
			if slices.Contains(avail, name) {
				return "/usr/bin/" + name, nil
			}
			return "", errors.New("not found")
		},
		geteuid:  func() int { return euid },
		getenv:   func(key string) string { return env[key] },
		hasToken: nil,
	}
}

// fakePATH は docker と gh を PATH 上に見せる。Executor は Fake なので実行はされず、
// ホストに docker / gh があるかに左右されずに Detect（公開関数）を検証できる。
// トークンの経路も固定するため GH_TOKEN と SUDO_USER は空にする。
func fakePATH(t *testing.T) {
	t.Helper()

	dir := t.TempDir()
	for _, name := range []string{"docker", "gh"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatalf("準備に失敗: %v", err)
		}
	}
	t.Setenv("PATH", dir)
	t.Setenv(envGHToken, "")
	t.Setenv(confpath.EnvSudoUser, "")
}

// okResult は「応答した」と判定される結果。
func okResult() (exec.Result, error) {
	return exec.Result{Stdout: []byte("28.0.1\n"), Stderr: nil, ExitCode: 0}, nil
}

// cmdlines は Fake が記録したコマンド行を返す。
func cmdlines(f *exec.Fake) []string {
	calls := f.Calls()
	out := make([]string, len(calls))
	for i, c := range calls {
		out[i] = c.String()
	}
	return out
}

func TestDetectRoot(t *testing.T) {
	for _, tt := range []struct {
		name string
		euid int
		want bool
	}{
		{name: "root", euid: 0, want: true},
		{name: "非 root", euid: 1000, want: false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			p := testProbes(nil, nil, tt.euid)
			got := detect(context.Background(), exec.NewFake(), p, testTimeout)
			if got.Root != tt.want {
				t.Errorf("Root = %v, want %v", got.Root, tt.want)
			}
		})
	}
}

func TestDetectSystemdAndJournal(t *testing.T) {
	for _, tt := range []struct {
		name                  string
		avail                 []string
		wantSystemd, wantJrnl bool
	}{
		{name: "両方ある", avail: []string{"systemctl", "journalctl"}, wantSystemd: true, wantJrnl: true},
		{name: "systemctl のみ", avail: []string{"systemctl"}, wantSystemd: true},
		{name: "journalctl のみ", avail: []string{"journalctl"}, wantJrnl: true},
		{name: "両方ない", avail: nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			p := testProbes(tt.avail, nil, 1000)
			got := detect(context.Background(), exec.NewFake(), p, testTimeout)
			if got.Systemd != tt.wantSystemd || got.Journal != tt.wantJrnl {
				t.Errorf("Systemd/Journal = %v/%v, want %v/%v", got.Systemd, got.Journal, tt.wantSystemd, tt.wantJrnl)
			}
		})
	}
}

// docker が無いときはコマンドを発行しないこと（監査ログに無駄なレコードを残さない）。
func TestDetectDockerAbsentIssuesNoCommand(t *testing.T) {
	f := exec.NewFake()
	got := detect(context.Background(), f, testProbes(nil, nil, 1000), testTimeout)
	if got.Docker {
		t.Error("docker 不在なのに Docker = true")
	}
	if lines := cmdlines(f); len(lines) != 0 {
		t.Errorf("コマンドを発行している: %v", lines)
	}
}

func TestDetectDockerResults(t *testing.T) {
	for _, tt := range []struct {
		name string
		res  exec.Result
		err  error
		want bool
	}{
		{name: "応答あり", res: exec.Result{Stdout: []byte("28.0.1\n"), ExitCode: 0}, want: true},
		{name: "終了コード 1", res: exec.Result{ExitCode: 1}, want: false},
		{name: "終了コード 0 だが出力が空", res: exec.Result{Stdout: []byte(" \n"), ExitCode: 0}, want: false},
		{name: "実行自体が失敗", res: exec.Result{ExitCode: -1}, err: errors.New("boom"), want: false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := exec.NewFake()
			f.SetFunc(func(_ string, _ []string) (exec.Result, error) { return tt.res, tt.err })

			p := testProbes([]string{"docker"}, nil, 1000)
			got := detect(context.Background(), f, p, testTimeout)
			if got.Docker != tt.want {
				t.Errorf("Docker = %v, want %v", got.Docker, tt.want)
			}
			want := []string{"docker info --format " + dockerServerVersionFormat}
			if lines := cmdlines(f); !slices.Equal(lines, want) {
				t.Errorf("発行コマンド = %v, want %v", lines, want)
			}
			if action := f.Calls()[0].Options.Action; action != "caps.docker" {
				t.Errorf("action = %q, want %q", action, "caps.docker")
			}
		})
	}
}

func TestDetectSudoUser(t *testing.T) {
	for _, tt := range []struct {
		name, env, want string
	}{
		{name: "正常", env: "ousiass", want: "ousiass"},
		{name: "未設定", env: "", want: ""},
		{name: "不正な文字種", env: "-x", want: ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			p := testProbes(nil, map[string]string{confpath.EnvSudoUser: tt.env}, 1000)
			if got := detect(context.Background(), exec.NewFake(), p, testTimeout).SudoUser; got != tt.want {
				t.Errorf("SudoUser = %q, want %q", got, tt.want)
			}
		})
	}
}

// Detect が Options を probes に渡し、HasToken を渡したら既定実装（gh / sudo）が
// 動かないこと。PATH を固定して、ホストに gh があるかに依存せず検証する。
func TestDetectWiresOptions(t *testing.T) {
	fakePATH(t)
	f := exec.NewFake()
	f.SetFunc(func(_ string, _ []string) (exec.Result, error) { return exec.Result{ExitCode: 1}, nil })

	opts := Options{
		HasToken: func(_ context.Context, _ exec.Executor) bool { return true },
		Timeout:  testTimeout,
	}
	got := Detect(context.Background(), f, opts)
	if !got.GitHubToken {
		t.Error("Options.HasToken の結果が Caps に反映されていない")
	}
	for _, c := range f.Calls() {
		if c.Name == "gh" || c.Name == "sudo" {
			t.Errorf("トークン取得の既定実装が動いている: %v", c)
		}
	}
}

// Detect が Options.Timeout をコマンドの期限に反映し、全体では detectBudget で
// 打ち切ること。Fake は Call に deadline を記録するので、これだけで検証できる。
func TestDetectAppliesTimeout(t *testing.T) {
	for _, tt := range []struct {
		name    string
		timeout time.Duration
		want    time.Duration
	}{
		{name: "Options.Timeout が 1 コマンドの上限になる", timeout: 300 * time.Millisecond, want: 300 * time.Millisecond},
		{name: "長すぎる指定は全体予算で打ち切る", timeout: time.Hour, want: detectBudget},
	} {
		t.Run(tt.name, func(t *testing.T) {
			fakePATH(t)
			f := exec.NewFake()
			f.SetFunc(func(_ string, _ []string) (exec.Result, error) { return okResult() })

			Detect(context.Background(), f, Options{HasToken: nil, Timeout: tt.timeout})
			calls := f.Calls()
			if len(calls) == 0 {
				t.Fatal("コマンドが発行されていない")
			}
			for _, c := range calls {
				if !c.HasDeadline {
					t.Fatalf("期限のない ctx で実行している: %v", c)
				}
				if remaining := time.Until(c.Deadline); remaining <= 0 || remaining > tt.want {
					t.Errorf("%v の残り時間 = %v, want 0 < x <= %v", c, remaining, tt.want)
				}
			}
		})
	}
}

// docker とトークンの判定は並行実行すること。逐次だと最悪 3 本分待つことになり
// 起動時間の目標に収まらない。実時間ではなく「2 本が同時に走っていること」で見る。
func TestDetectRunsProbesConcurrently(t *testing.T) {
	f := exec.NewFake()
	arrived := make(chan struct{}, 2)
	release := make(chan struct{})
	f.SetFunc(func(_ string, _ []string) (exec.Result, error) {
		arrived <- struct{}{}
		<-release // 2 本目が来るまで返さない。逐次実装ならここで詰まる
		return okResult()
	})

	done := make(chan Caps, 1)
	go func() {
		done <- detect(context.Background(), f, testProbes([]string{"docker", "gh"}, nil, 1000), testTimeout)
	}()

	for i := 1; i <= 2; i++ {
		select {
		case <-arrived:
		case <-time.After(testTimeout):
			close(release)
			t.Fatalf("%d 本目のコマンドが発行されない（逐次実行になっている）", i)
		}
	}
	close(release)

	if got := <-done; !got.Docker || !got.GitHubToken {
		t.Errorf("Docker/GitHubToken = %v/%v, want true/true", got.Docker, got.GitHubToken)
	}
}

// Timeout 未指定なら既定値が使われること。
func TestProbeTimeout(t *testing.T) {
	for _, tt := range []struct {
		name string
		in   time.Duration
		want time.Duration
	}{
		{name: "未指定", in: 0, want: defaultProbeTimeout},
		{name: "負値", in: -time.Second, want: defaultProbeTimeout},
		{name: "指定あり", in: testTimeout, want: testTimeout},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := probeTimeout(tt.in); got != tt.want {
				t.Errorf("probeTimeout(%v) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}
