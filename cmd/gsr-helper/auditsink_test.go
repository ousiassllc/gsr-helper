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

// クローズの失敗は 1 行の警告として出す。捨てると、開けなかった場合と記録に
// 失敗した場合は警告が出るのに、閉じ損ないだけが利用者に見えないままになる。
func TestReportAuditClose(t *testing.T) {
	var buf bytes.Buffer
	reportAuditClose(nil, &buf)
	if buf.Len() != 0 {
		t.Errorf("失敗が無いのに出力している: %q", buf.String())
	}

	buf.Reset()
	reportAuditClose(errors.New("監査ログのクローズに失敗しました: ディスクが満杯です"), &buf)
	got := buf.String()
	want := "警告: 監査ログのクローズに失敗しました: ディスクが満杯です\n"
	if got != want {
		t.Errorf("出力 = %q, want %q", got, want)
	}
	if strings.Count(got, "\n") != 1 {
		t.Errorf("1 行で出していない: %q", got)
	}
}
