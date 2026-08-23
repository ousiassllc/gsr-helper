package authz_test

import (
	"os"
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/doctor/check"
	"github.com/ousiassllc/gsr-helper/internal/runner"
	"github.com/ousiassllc/gsr-helper/internal/runner/procs"
)

// .credentials は所有者だけが読める状態でなければならない。
func TestPermJudgesCredentialsMode(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		modes map[string]os.FileMode
		want  check.Status
	}{
		"0600 と 0644 は適切": {
			modes: map[string]os.FileMode{".credentials": 0o600, ".runner": 0o644},
			want:  check.OK,
		},
		"credentials が group 読み取り可": {
			modes: map[string]os.FileMode{".credentials": 0o640, ".runner": 0o644},
			want:  check.Fail,
		},
		"credentials が other 読み取り可": {
			modes: map[string]os.FileMode{".credentials": 0o604, ".runner": 0o644},
			want:  check.Fail,
		},
		"runner が world-writable": {
			modes: map[string]os.FileMode{".credentials": 0o600, ".runner": 0o666},
			want:  check.Warn,
		},
		"両方とも無い runner は未設定": {
			modes: nil,
			want:  check.Skip,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			runnerDir(t, root, "/opt/runners/build01", tt.modes)
			in := check.Input{Runners: []runner.Runner{newRunner("build01")}, FSRoot: root}

			got := only(t, run(t, "authz.perm", in))
			if got.Status != tt.want {
				t.Errorf("Status = %v, want %v（Detail: %s）", got.Status, tt.want, got.Detail)
			}
			if got.Target != "build01" {
				t.Errorf("Target = %q, want %q", got.Target, "build01")
			}
		})
	}
}

// 所有者が runner の実行ユーザーと違えば注意を出す。
func TestPermJudgesOwner(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	runnerDir(t, root, "/opt/runners/build01", map[string]os.FileMode{
		".credentials": 0o600, ".runner": 0o644,
	})

	r := newRunner("build01")
	// 実ファイルの所有者は必ずテスト実行ユーザーなので、別の UID を持つ
	// Listener を与えれば食い違いを作れる。
	r.Listener = &procs.Process{PID: 1, Kind: procs.Listener, Dir: r.Dir, UID: os.Getuid() + 1}

	got := only(t, run(t, "authz.perm", check.Input{Runners: []runner.Runner{r}, FSRoot: root}))
	if got.Status != check.Warn {
		t.Errorf("Status = %v, want %v（Detail: %s）", got.Status, check.Warn, got.Detail)
	}
}

// runner が 1 台も無ければ SKIP。
func TestPermWithoutRunners(t *testing.T) {
	t.Parallel()

	if got := only(t, run(t, "authz.perm", check.Input{})).Status; got != check.Skip {
		t.Errorf("Status = %v, want %v", got, check.Skip)
	}
}
