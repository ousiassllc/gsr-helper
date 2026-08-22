package main

import (
	"bytes"
	"errors"
	"strings"
	"sync"
	"testing"
)

func TestAuditSinkReportsNothingWhenNoFailure(t *testing.T) {
	var buf bytes.Buffer
	s := &auditSink{}
	s.add(nil) // nil は失敗ではない
	s.report(&buf)
	if buf.Len() != 0 {
		t.Errorf("失敗が無いのに出力している: %q", buf.String())
	}
}

func TestAuditSinkReportsCountAndFirstError(t *testing.T) {
	var buf bytes.Buffer
	s := &auditSink{}
	s.add(errors.New("最初の失敗"))
	s.add(errors.New("二度目の失敗"))
	s.report(&buf)

	out := buf.String()
	if !strings.Contains(out, "2 件") {
		t.Errorf("件数が出ていない: %q", out)
	}
	if !strings.Contains(out, "最初の失敗") {
		t.Errorf("最初の失敗が出ていない: %q", out)
	}
	if strings.Contains(out, "二度目の失敗") {
		t.Errorf("2 件目まで並べている: %q", out)
	}
}

// Executor は複数の goroutine から実行されるため、通知も並行して届く。
// -race で実行することを前提にした検証。
func TestAuditSinkIsSafeForConcurrentUse(t *testing.T) {
	const n = 8
	s := &auditSink{}

	var wg sync.WaitGroup
	for range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.add(errors.New("失敗"))
		}()
	}
	wg.Wait()

	var buf bytes.Buffer
	s.report(&buf)
	if !strings.Contains(buf.String(), "8 件") {
		t.Errorf("件数が合わない: %q", buf.String())
	}
}
