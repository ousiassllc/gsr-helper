package authz_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/doctor/check"
)

// hidepid は 2 でなければ注意を出す。トークンが /proc から読めるためである。
func TestHidepid(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		mounts string
		want   check.Status
	}{
		"hidepid=2": {
			mounts: "proc /proc proc rw,nosuid,nodev,noexec,relatime,hidepid=2 0 0\n",
			want:   check.OK,
		},
		"hidepid=invisible": {
			mounts: "proc /proc proc rw,hidepid=invisible 0 0\n",
			want:   check.OK,
		},
		"hidepid=1": {
			mounts: "proc /proc proc rw,hidepid=1 0 0\n",
			want:   check.Warn,
		},
		"設定なし": {
			mounts: "proc /proc proc rw,nosuid,nodev,noexec,relatime 0 0\n",
			want:   check.Warn,
		},
		"proc の行が無い": {
			mounts: "/dev/sda1 / ext4 rw 0 0\n",
			want:   check.Skip,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			path := filepath.Join(root, "proc", "mounts")
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatalf("MkdirAll: %v", err)
			}
			if err := os.WriteFile(path, []byte(tt.mounts), 0o600); err != nil {
				t.Fatalf("WriteFile: %v", err)
			}

			got := only(t, run(t, "authz.hidepid", check.Input{FSRoot: root}))
			if got.Status != tt.want {
				t.Errorf("Status = %v, want %v（Detail: %s）", got.Status, tt.want, got.Detail)
			}
		})
	}
}

// /proc/mounts が読めなければ SKIP。
func TestHidepidWithoutMounts(t *testing.T) {
	t.Parallel()

	got := only(t, run(t, "authz.hidepid", check.Input{FSRoot: t.TempDir()}))
	if got.Status != check.Skip {
		t.Errorf("Status = %v, want %v", got.Status, check.Skip)
	}
}
