package hostres_test

import (
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/doctor/check"
)

func TestMemory(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		meminfo string
		want    check.Status
	}{
		"余裕がある": {
			meminfo: "MemTotal: 1000 kB\nMemAvailable: 800 kB\nSwapTotal: 500 kB\nSwapFree: 500 kB\n",
			want:    check.OK,
		},
		"空きが 20% を切る": {
			meminfo: "MemTotal: 1000 kB\nMemAvailable: 150 kB\nSwapTotal: 500 kB\nSwapFree: 500 kB\n",
			want:    check.Warn,
		},
		"空きが 10% を切る": {
			meminfo: "MemTotal: 1000 kB\nMemAvailable: 50 kB\nSwapTotal: 500 kB\nSwapFree: 500 kB\n",
			want:    check.Fail,
		},
		"余裕はあるが swap が無い": {
			meminfo: "MemTotal: 1000 kB\nMemAvailable: 800 kB\nSwapTotal: 0 kB\nSwapFree: 0 kB\n",
			want:    check.Warn,
		},
		"必要な項目が無い": {
			meminfo: "MemTotal: 1000 kB\n",
			want:    check.Skip,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			in := check.Input{FSRoot: procFS(t, "meminfo", tt.meminfo)}
			got := only(t, run(t, "resource.mem", in))
			if got.Status != tt.want {
				t.Errorf("Status = %v, want %v（Detail: %s）", got.Status, tt.want, got.Detail)
			}
		})
	}
}

// /proc/meminfo を読めなければ SKIP。
func TestMemoryWithoutMeminfo(t *testing.T) {
	t.Parallel()

	if got := only(t, run(t, "resource.mem", check.Input{FSRoot: t.TempDir()})).Status; got != check.Skip {
		t.Errorf("Status = %v, want %v", got, check.Skip)
	}
}
