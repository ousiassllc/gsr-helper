package command

import (
	"bytes"
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/exec"
)

// concurrency は並行テストで走らせる goroutine 数。
const concurrency = 8

func TestCommandRunConcurrentlySharesAuditLogger(t *testing.T) {
	// 1 つの Logger を複数の goroutine から共有しても、1 レコードが 1 行として
	// 書かれ、行が混ざらないことを Run 経路で確かめる。
	var buf bytes.Buffer
	c := New(NoSecrets, WithAudit(testLogger(&buf)))

	var wg sync.WaitGroup
	for i := range concurrency {
		wg.Add(1)
		go func() {
			defer wg.Done()
			name, args := helperCommand()
			ctx := exec.WithOptions(context.Background(), exec.Options{
				Action: "svc.stop",
				Runner: "build01-" + strconv.Itoa(i),
				Env:    helperEnv(helperStdoutEnv + "=ok"),
			})
			if _, err := c.Run(ctx, name, args...); err != nil {
				t.Errorf("Run がエラーを返した: %v", err)
			}
		}()
	}
	wg.Wait()

	lines := strings.Split(strings.TrimSuffix(buf.String(), "\n"), "\n")
	if len(lines) != concurrency {
		t.Fatalf("行数 = %d, want %d: %q", len(lines), concurrency, buf.String())
	}

	runners := make(map[string]bool, concurrency)
	for _, line := range lines {
		var rec auditLine
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("監査レコードが読めない（行が混ざっている）: %v (%s)", err, line)
		}
		if rec.Action != "svc.stop" || rec.ExitCode != 0 {
			t.Errorf("レコードが想定と異なる: %+v", rec)
		}
		runners[rec.Runner] = true
	}
	if len(runners) != concurrency {
		t.Errorf("記録された runner の種類 = %d, want %d", len(runners), concurrency)
	}
}
