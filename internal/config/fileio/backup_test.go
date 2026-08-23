package fileio_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/config/fileio"
)

// バックアップが内容とパーミッションを引き継ぐこと（FR-38）。
// バックアップだけが 0600 で残ると、利用者が自分で戻せなくなる。
func TestBackupCopiesContentAndMode(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), ".env")
	write(t, path, "A=1\n# コメント\n", 0o640)

	if err := fileio.Backup(path); err != nil {
		t.Fatalf("Backup() でエラー: %v", err)
	}

	dst := path + fileio.BackupSuffix
	if got := read(t, dst); got != "A=1\n# コメント\n" {
		t.Errorf("バックアップの内容 = %q, want %q", got, "A=1\n# コメント\n")
	}

	fi, err := os.Lstat(dst)
	if err != nil {
		t.Fatalf("Lstat() でエラー: %v", err)
	}
	if got := fi.Mode().Perm(); got != 0o640 {
		t.Errorf("バックアップのパーミッション = %v, want %v", got, fs.FileMode(0o640))
	}

	// 元のファイルは残る。
	if got := read(t, path); got != "A=1\n# コメント\n" {
		t.Errorf("元のファイルが変わっている: %q", got)
	}
}

// 既存のバックアップは置き換えること。世代は持たない。
func TestBackupReplacesExisting(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), ".env")
	write(t, path, "新しい\n", 0o600)
	write(t, path+fileio.BackupSuffix, "古い\n", 0o600)

	if err := fileio.Backup(path); err != nil {
		t.Fatalf("Backup() でエラー: %v", err)
	}
	if got := read(t, path+fileio.BackupSuffix); got != "新しい\n" {
		t.Errorf("バックアップ = %q, want %q", got, "新しい\n")
	}
}

// 元ファイルが無い場合は fs.ErrNotExist を包んで返すこと。
// 呼び出し側が「まだ元ファイルが無いだけ」を判別できる必要がある。
func TestBackupMissingSource(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), ".env")
	if err := fileio.Backup(path); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("Backup() のエラー = %v, want fs.ErrNotExist", err)
	}
}

// 元ファイルがシンボリックリンクなら拒むこと。
func TestBackupRefusesSymlinkSource(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	victim := filepath.Join(dir, "victim")
	write(t, victim, "秘密\n", 0o600)

	link := filepath.Join(dir, ".env")
	if err := os.Symlink(victim, link); err != nil {
		t.Fatalf("シンボリックリンクの作成に失敗: %v", err)
	}

	if err := fileio.Backup(link); !errors.Is(err, fileio.ErrSymlink) {
		t.Fatalf("Backup() のエラー = %v, want ErrSymlink", err)
	}
	if _, err := os.Lstat(link + fileio.BackupSuffix); !errors.Is(err, fs.ErrNotExist) {
		t.Error("拒んだのにバックアップが作られている")
	}
}

// バックアップ先がシンボリックリンクなら拒むこと。
// リンクだったという事実を黙って消さないための判定である。
func TestBackupRefusesSymlinkDestination(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	write(t, path, "A=1\n", 0o600)

	victim := filepath.Join(dir, "victim")
	write(t, victim, "守るべき内容\n", 0o600)
	if err := os.Symlink(victim, path+fileio.BackupSuffix); err != nil {
		t.Fatalf("シンボリックリンクの作成に失敗: %v", err)
	}

	if err := fileio.Backup(path); !errors.Is(err, fileio.ErrSymlink) {
		t.Fatalf("Backup() のエラー = %v, want ErrSymlink", err)
	}
	if got := read(t, victim); got != "守るべき内容\n" {
		t.Errorf("リンク先が書き換えられた: %q", got)
	}
}
