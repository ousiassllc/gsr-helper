package hostres_test

import (
	"strings"
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/doctor/check"
	"github.com/ousiassllc/gsr-helper/internal/exec"
)

func TestNTP(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		stdout string
		look   func(string) (string, error)
		want   check.Status
	}{
		"同期済み": {
			stdout: "NTP=yes\nNTPSynchronized=yes\nTimeUSec=Sat 2026-08-23 12:00:00 JST\n",
			look:   lookOnly("timedatectl"), want: check.OK,
		},
		"未同期": {
			stdout: "NTP=yes\nNTPSynchronized=no\nTimeUSec=Sat 2026-08-23 12:00:00 JST\n",
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
				Exec:     fakeExec(func(string, []string) (exec.Result, error) { return okResult(tt.stdout), nil }),
				LookPath: tt.look,
			}
			got := only(t, run(t, "time.ntp", in))
			if got.Status != tt.want {
				t.Errorf("Status = %v, want %v（Detail: %s）", got.Status, tt.want, got.Detail)
			}
		})
	}
}

// ずれの秒数は外部の基準時刻が要るため出さない。出さない理由を Detail に書く。
func TestNTPDoesNotClaimDriftSeconds(t *testing.T) {
	t.Parallel()

	in := check.Input{
		Exec: fakeExec(func(string, []string) (exec.Result, error) {
			return okResult("NTP=yes\nNTPSynchronized=no\nTimeUSec=x\n"), nil
		}),
		LookPath: lookOnly("timedatectl"),
	}
	got := only(t, run(t, "time.ntp", in))
	if !strings.Contains(got.Detail, "ずれの秒数") {
		t.Errorf("ずれを測っていない旨が Detail に無い: %s", got.Detail)
	}
	if !strings.Contains(got.Remedy, "timedatectl set-ntp true") {
		t.Errorf("Remedy に同期を有効にする手順が無い: %s", got.Remedy)
	}
}
