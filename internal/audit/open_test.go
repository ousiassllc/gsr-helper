package audit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// mode は path のパーミッションを返す。
func mode(t *testing.T, path string) os.FileMode {
	t.Helper()
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatalf("%s の Stat に失敗した: %v", path, err)
	}
	return fi.Mode().Perm()
}

func TestOpenCreatesDirAndFileWithTightMode(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "gsr-helper")
	path := filepath.Join(dir, "audit.jsonl")

	lg, err := Open(path)
	if err != nil {
		t.Fatalf("Open がエラーを返した: %v", err)
	}
	defer func() { _ = lg.Close() }()

	if got := mode(t, dir); got != 0o700 {
		t.Errorf("ディレクトリのモード = %#o, want 0o700", got)
	}
	if got := mode(t, path); got != 0o600 {
		t.Errorf("ファイルのモード = %#o, want 0o600", got)
	}
}

func TestOpenKeepsExistingFileMode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.jsonl")
	if err := os.WriteFile(path, []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("事前のファイル作成に失敗した: %v", err)
	}

	lg, err := Open(path)
	if err != nil {
		t.Fatalf("Open がエラーを返した: %v", err)
	}
	defer func() { _ = lg.Close() }()

	// 自分が作っていないファイルは Chmod しない（緩いまま使う方が、root による
	// 任意ファイルの再パーミッションを許すより安全）。
	if got := mode(t, path); got != 0o644 {
		t.Errorf("既存ファイルのモード = %#o, want 0o644（変更しない）", got)
	}
}

func TestOpenKeepsExistingDirMode(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "existing")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatalf("事前のディレクトリ作成に失敗した: %v", err)
	}

	lg, err := Open(filepath.Join(dir, "audit.jsonl"))
	if err != nil {
		t.Fatalf("Open がエラーを返した: %v", err)
	}
	defer func() { _ = lg.Close() }()

	// /var/log 配下のような既存ディレクトリの権限を勝手に変えない。
	if got := mode(t, dir); got != 0o755 {
		t.Errorf("既存ディレクトリのモード = %#o, want 0o755（変更しない）", got)
	}
}

func TestOpenAppends(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.jsonl")

	for _, action := range []string{"svc.stop", "svc.start"} {
		lg, err := Open(path)
		if err != nil {
			t.Fatalf("Open がエラーを返した: %v", err)
		}
		if err := lg.Write(Record{Action: action}); err != nil {
			t.Fatalf("Write がエラーを返した: %v", err)
		}
		if err := lg.Close(); err != nil {
			t.Fatalf("Close がエラーを返した: %v", err)
		}
	}

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("読み込みに失敗した: %v", err)
	}
	lines := strings.Split(strings.TrimSuffix(string(b), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("行数 = %d, want 2（追記されていない）", len(lines))
	}
	if !strings.Contains(lines[0], "svc.stop") || !strings.Contains(lines[1], "svc.start") {
		t.Errorf("追記の順序が想定と異なる: %q", lines)
	}
}

func TestOpenErrorOnUnwritablePath(t *testing.T) {
	// 親がファイルなので MkdirAll が失敗する。
	blocker := filepath.Join(t.TempDir(), "notadir")
	if err := os.WriteFile(blocker, nil, 0o600); err != nil {
		t.Fatalf("事前のファイル作成に失敗した: %v", err)
	}

	if _, err := Open(filepath.Join(blocker, "sub", "audit.jsonl")); err == nil {
		t.Fatal("書けないパスでエラーを返していない")
	}
}

func TestOpenCloseClosesFile(t *testing.T) {
	lg, err := Open(filepath.Join(t.TempDir(), "audit.jsonl"))
	if err != nil {
		t.Fatalf("Open がエラーを返した: %v", err)
	}
	if err := lg.Close(); err != nil {
		t.Fatalf("Close がエラーを返した: %v", err)
	}
	// 二重の Close は no-op。
	if err := lg.Close(); err != nil {
		t.Fatalf("2 回目の Close がエラーを返した: %v", err)
	}
	if err := lg.Write(Record{Action: "svc.stop"}); err == nil {
		t.Error("Close 後の Write がエラーを返していない")
	}
}

func TestOpenRejectsSymlink(t *testing.T) {
	// 本ツールは sudo 前提で動くため、先置きされたシンボリックリンク経由で
	// 任意ファイルへの Chmod 0600 と追記を代行させられてはならない。
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	if err := os.WriteFile(target, nil, 0o644); err != nil {
		t.Fatalf("リンク先の作成に失敗した: %v", err)
	}

	path := filepath.Join(dir, "audit.jsonl")
	if err := os.Symlink(target, path); err != nil {
		t.Fatalf("シンボリックリンクの作成に失敗した: %v", err)
	}

	lg, err := Open(path)
	if err == nil {
		_ = lg.Close()
		t.Fatal("シンボリックリンクを開いてしまった")
	}
	if got := mode(t, target); got != 0o644 {
		t.Errorf("リンク先のモード = %#o, want 0o644（変更してはいけない）", got)
	}
	b, rerr := os.ReadFile(target)
	if rerr != nil {
		t.Fatalf("リンク先の読み込みに失敗した: %v", rerr)
	}
	if len(b) != 0 {
		t.Errorf("リンク先に書き込まれている: %q", b)
	}
}

func TestOpenRejectsSymlinkedDir(t *testing.T) {
	// 最終要素がディレクトリへのリンク配下にある場合は、リンク自体は最終要素では
	// ないため通常どおり開ける（既存ディレクトリの扱いを変えないことの確認）。
	root := t.TempDir()
	realDir := filepath.Join(root, "real")
	if err := os.Mkdir(realDir, 0o700); err != nil {
		t.Fatalf("ディレクトリ作成に失敗した: %v", err)
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(realDir, link); err != nil {
		t.Fatalf("シンボリックリンクの作成に失敗した: %v", err)
	}

	lg, err := Open(filepath.Join(link, "audit.jsonl"))
	if err != nil {
		t.Fatalf("Open がエラーを返した: %v", err)
	}
	if err := lg.Close(); err != nil {
		t.Fatalf("Close がエラーを返した: %v", err)
	}
	if got := mode(t, filepath.Join(realDir, "audit.jsonl")); got != 0o600 {
		t.Errorf("ファイルのモード = %#o, want 0o600", got)
	}
}
