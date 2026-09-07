package name_test

import (
	"slices"
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/setup/name"
)

func TestNextIndex(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		existing []string
		prefix   string
		want     int
	}{
		"既存なし":                {nil, "build01", 1},
		"1 台だけ":               {[]string{"build01-1"}, "build01", 2},
		"最大値の次を返す":            {[]string{"build01-1", "build01-3", "build01-2"}, "build01", 4},
		"欠番があっても最大値の次":        {[]string{"build01-1", "build01-7"}, "build01", 8},
		"別の接頭辞は数えない":          {[]string{"web01-9", "build01-2"}, "build01", 3},
		"接頭辞だけの名前は数えない":       {[]string{"build01"}, "build01", 1},
		"連番でない末尾は数えない":        {[]string{"build01-abc", "build01-1a"}, "build01", 1},
		"ゼロ埋めは数として読む":         {[]string{"build01-007"}, "build01", 8},
		"負の値らしい名前は数えない":       {[]string{"build01--1"}, "build01", 1},
		"0 は連番として扱わない":        {[]string{"build01-0"}, "build01", 1},
		"接頭辞が空なら 1":           {[]string{"build01-5"}, "", 1},
		"接頭辞にハイフンを含んでも正しく数える": {[]string{"gpu-box-4"}, "gpu-box", 5},
		"入れ子の連番を誤って拾わない":      {[]string{"build01-1-2"}, "build01", 1},
	}

	for label, tt := range tests {
		t.Run(label, func(t *testing.T) {
			t.Parallel()
			if got := name.NextIndex(tt.existing, tt.prefix); got != tt.want {
				t.Errorf("NextIndex(%v, %q) = %d, want %d", tt.existing, tt.prefix, got, tt.want)
			}
		})
	}
}

func TestNames(t *testing.T) {
	t.Parallel()

	got := name.Names("build01", 5, 3)
	want := []string{"build01-5", "build01-6", "build01-7"}
	if !slices.Equal(got, want) {
		t.Errorf("Names = %v, want %v", got, want)
	}
}
