package exec

import (
	"context"
	"strconv"
	"sync"
	"testing"
)

// concurrency は並行テストで走らせる goroutine 数。
const concurrency = 8

func TestFakeIsSafeForConcurrentUse(t *testing.T) {
	// Fake は mutex を持つため、Run と Calls を同時に叩いても壊れない。
	// -race 下で実行することを前提にした検証。
	f := NewFake()
	f.SetFunc(func(string, []string) (Result, error) { return Result{Stdout: []byte("ok")}, nil })

	var wg sync.WaitGroup
	for i := range concurrency {
		wg.Add(2)
		go func() {
			defer wg.Done()
			if _, err := f.Run(context.Background(), "systemctl", "stop", strconv.Itoa(i)); err != nil {
				t.Errorf("Run がエラーを返した: %v", err)
			}
		}()
		go func() {
			defer wg.Done()
			for _, c := range f.Calls() {
				if c.Name != "systemctl" {
					t.Errorf("Name = %q, want systemctl", c.Name)
				}
			}
		}()
	}
	wg.Wait()

	if got := len(f.Calls()); got != concurrency {
		t.Errorf("記録件数 = %d, want %d", got, concurrency)
	}
}
