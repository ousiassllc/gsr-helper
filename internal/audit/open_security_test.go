package audit

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

// 本ツールは sudo で動き、audit_log のパスは非特権ユーザーが持つ設定ファイル
// 由来である。既存ファイルを掴んだときに root が Chmod や追記を代行しないことを
// ここで固定する。

func TestOpenRejectsForeignExistingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.jsonl")
	const content = "root:x:0:0:root:/root:/bin/bash\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("事前のファイル作成に失敗した: %v", err)
	}

	lg, err := Open(path)
	if err == nil {
		_ = lg.Close()
		t.Fatal("監査ログではないファイルを開いてしまった")
	}

	if got := mode(t, path); got != 0o644 {
		t.Errorf("モード = %#o, want 0o644（変更してはいけない）", got)
	}
	b, rerr := os.ReadFile(path)
	if rerr != nil {
		t.Fatalf("読み込みに失敗した: %v", rerr)
	}
	if string(b) != content {
		t.Errorf("内容 = %q, want %q（追記してはいけない）", b, content)
	}
}

func TestOpenRejectsHardLink(t *testing.T) {
	dir := t.TempDir()
	// 攻撃者が別名から張ったハードリンク。追記先が監査ログ以外にも見えるため拒む。
	target := filepath.Join(dir, "target.jsonl")
	if err := os.WriteFile(target, []byte("{}\n"), 0o600); err != nil {
		t.Fatalf("リンク元の作成に失敗した: %v", err)
	}
	path := filepath.Join(dir, "audit.jsonl")
	if err := os.Link(target, path); err != nil {
		t.Fatalf("ハードリンクの作成に失敗した: %v", err)
	}

	lg, err := Open(path)
	if err == nil {
		_ = lg.Close()
		t.Fatal("ハードリンクを開いてしまった")
	}
}

func TestOpenAcceptsEmptyExistingFile(t *testing.T) {
	// 前回の実行が 1 行も書かずに終わった場合など、空ファイルは正常な状態。
	path := filepath.Join(t.TempDir(), "audit.jsonl")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatalf("事前のファイル作成に失敗した: %v", err)
	}

	lg, err := Open(path)
	if err != nil {
		t.Fatalf("Open がエラーを返した: %v", err)
	}
	if err := lg.Write(Record{Action: "svc.stop"}); err != nil {
		t.Fatalf("Write がエラーを返した: %v", err)
	}
	if err := lg.Close(); err != nil {
		t.Fatalf("Close がエラーを返した: %v", err)
	}
}

func TestOpenAcceptsExistingAuditFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.jsonl")
	if err := os.WriteFile(path, []byte("{\"action\":\"svc.stop\"}\n"), 0o600); err != nil {
		t.Fatalf("事前のファイル作成に失敗した: %v", err)
	}

	lg, err := Open(path)
	if err != nil {
		t.Fatalf("Open がエラーを返した: %v", err)
	}
	if err := lg.Close(); err != nil {
		t.Fatalf("Close がエラーを返した: %v", err)
	}
}

func TestOpenRejectsFifo(t *testing.T) {
	// 通常ファイル以外は書き込みがブロックしたり別プロセスに読まれたりする。
	path := filepath.Join(t.TempDir(), "audit.jsonl")
	if err := mkfifo(path); err != nil {
		t.Skipf("FIFO を作れないためスキップする: %v", err)
	}

	lg, err := Open(path)
	if err == nil {
		_ = lg.Close()
		t.Fatal("FIFO を開いてしまった")
	}
}

// mkfifo は path に名前付きパイプを作る。
func mkfifo(path string) error {
	return syscall.Mkfifo(path, 0o600)
}
