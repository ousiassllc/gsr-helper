package disk

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/audit"
	"github.com/ousiassllc/gsr-helper/internal/exec"
)

// Issue #71: ファイルの再帰削除は internal/exec を通らないため、記録の起点を
// internal/disk 側（removeTarget）にも持つ。ここではその記録内容を検証する。

// decodeAuditLines は buf に書かれた JSON Lines を Record として読み戻す。
func decodeAuditLines(t *testing.T, buf *bytes.Buffer) []audit.Record {
	t.Helper()

	var recs []audit.Record
	for _, line := range bytes.Split(bytes.TrimRight(buf.Bytes(), "\n"), []byte("\n")) {
		if len(line) == 0 {
			continue
		}
		var rec audit.Record
		if err := json.Unmarshal(line, &rec); err != nil {
			t.Fatalf("監査ログの行が JSON として読めない: %v (%s)", err, line)
		}
		recs = append(recs, rec)
	}
	return recs
}

// TestApplyRecordsAuditPerRemovedTarget は削除 1 件につき 1 レコードが出ること、
// 対象パスが command に載ること、成功時に exit_code が 0 になることを確かめる。
func TestApplyRecordsAuditPerRemovedTarget(t *testing.T) {
	dir := newTargetTree(t)
	mkFile(t, filepath.Join(dir, "_diag", "a.log"), 2)

	targets := []Target{pathTarget(dir, "_diag", 2), pathTarget(dir, filepath.Join("_work", "_temp"), 0)}
	plan, err := PlanClean(targets)
	if err != nil {
		t.Fatalf("PlanClean がエラーを返した: %v", err)
	}

	var buf bytes.Buffer
	lg := audit.New(&buf)
	if aerr := Apply(context.Background(), exec.NewFake(), lg, plan, nil); aerr != nil {
		t.Fatalf("Apply がエラーを返した: %v", aerr)
	}

	recs := decodeAuditLines(t, &buf)
	if len(recs) != len(targets) {
		t.Fatalf("レコード数 = %d, want %d（対象ごとに 1 件）", len(recs), len(targets))
	}
	for i, rec := range recs {
		if rec.Action != actionClean {
			t.Errorf("[%d] Action = %q, want %q", i, rec.Action, actionClean)
		}
		if rec.Runner != targets[i].Runner {
			t.Errorf("[%d] Runner = %q, want %q", i, rec.Runner, targets[i].Runner)
		}
		if rec.Dir != targets[i].Base {
			t.Errorf("[%d] Dir = %q, want %q", i, rec.Dir, targets[i].Base)
		}
		wantCommand := []string{auditRemoveLabel, targets[i].Path}
		if len(rec.Command) != 2 || rec.Command[0] != wantCommand[0] || rec.Command[1] != wantCommand[1] {
			t.Errorf("[%d] Command = %+v, want %+v", i, rec.Command, wantCommand)
		}
		if rec.ExitCode != 0 {
			t.Errorf("[%d] ExitCode = %d, want 0（成功）", i, rec.ExitCode)
		}
		if rec.Error != "" {
			t.Errorf("[%d] Error = %q, want 空（成功）", i, rec.Error)
		}
	}
}

// TestApplyRecordsAuditOnAbortedTarget は保護・検証で中止した対象も
// ExitCode: 1 と Error 付きで 1 レコード記録されることを確かめる。
// 削除しなかった事実が監査ログから後で追える必要がある（受け入れ条件）。
func TestApplyRecordsAuditOnAbortedTarget(t *testing.T) {
	dir := newTargetTree(t)
	mkFile(t, filepath.Join(dir, "_work", "_temp", "a.txt"), 3)
	busy := protectedTarget(dir, filepath.Join("_work", "_temp"))

	plan := CleanPlan{Paths: []Target{busy}, Docker: false, Bytes: 100, Commands: nil}

	var buf bytes.Buffer
	lg := audit.New(&buf)
	if err := Apply(context.Background(), exec.NewFake(), lg, plan, nil); err == nil {
		t.Fatal("Apply がエラーを返さなかった")
	}

	recs := decodeAuditLines(t, &buf)
	if len(recs) != 1 {
		t.Fatalf("レコード数 = %d, want 1", len(recs))
	}
	rec := recs[0]
	if rec.ExitCode != 1 {
		t.Errorf("ExitCode = %d, want 1（中止）", rec.ExitCode)
	}
	if rec.Error == "" {
		t.Error("Error が空。中止の理由が追えない")
	}
	if len(rec.Command) != 2 || rec.Command[1] != busy.Path {
		t.Errorf("Command = %+v, want パスを含む", rec.Command)
	}
	if !exists(filepath.Join(dir, "_work", "_temp", "a.txt")) {
		t.Error("保護された対象を削除してしまった")
	}
}

// TestApplyRunsWithoutAuditLogger は lg が nil でも削除自体は従来どおり
// 続くことを確かめる（監査ログを開けない場合の縮退）。
func TestApplyRunsWithoutAuditLogger(t *testing.T) {
	dir := newTargetTree(t)
	mkFile(t, filepath.Join(dir, "_diag", "a.log"), 2)

	plan, err := PlanClean([]Target{pathTarget(dir, "_diag", 2)})
	if err != nil {
		t.Fatalf("PlanClean がエラーを返した: %v", err)
	}

	if aerr := Apply(context.Background(), exec.NewFake(), nil, plan, nil); aerr != nil {
		t.Fatalf("Apply がエラーを返した: %v", aerr)
	}
	if exists(filepath.Join(dir, "_diag")) {
		t.Error("lg が nil でも削除が進むはずが、対象が残っている")
	}
}
