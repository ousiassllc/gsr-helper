package fileio_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/config/fileio"
)

// write は検証用に path へ内容を置く。
func write(t *testing.T, path, body string, mode fs.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), mode); err != nil {
		t.Fatalf("%s の作成に失敗: %v", path, err)
	}
	// WriteFile は umask の影響を受けるため、狙ったパーミッションに揃える。
	if err := os.Chmod(path, mode); err != nil {
		t.Fatalf("%s のパーミッション設定に失敗: %v", path, err)
	}
}

// read は path の内容を読む。
func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%s の読み込みに失敗: %v", path, err)
	}
	return string(b)
}

// 既存ファイルのパーミッションを引き継ぐこと。root で書き戻した結果 runner 自身が
// .env を読めなくなる事故を防ぐための性質であり、既定値で上書きしてはならない。
func TestWriteKeepsExistingMode(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), ".env")
	write(t, path, "A=1\n", 0o640)

	if err := fileio.Write(path, []byte("A=2\n"), 0o600); err != nil {
		t.Fatalf("Write() でエラー: %v", err)
	}

	fi, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("Lstat() でエラー: %v", err)
	}
	if got := fi.Mode().Perm(); got != 0o640 {
		t.Errorf("パーミッション = %v, want %v", got, fs.FileMode(0o640))
	}
	if got := read(t, path); got != "A=2\n" {
		t.Errorf("内容 = %q, want %q", got, "A=2\n")
	}
}

// 新規作成では引数のパーミッションを使うこと。
func TestWriteUsesNewModeForMissingFile(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), ".env")
	if err := fileio.Write(path, []byte("A=1\n"), 0o600); err != nil {
		t.Fatalf("Write() でエラー: %v", err)
	}

	fi, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("Lstat() でエラー: %v", err)
	}
	if got := fi.Mode().Perm(); got != 0o600 {
		t.Errorf("パーミッション = %v, want %v", got, fs.FileMode(0o600))
	}
}

// 書き込みに失敗しても元のファイルが壊れないこと（原子的な置き換え）。
// 一時ファイルを作れないディレクトリを用意して失敗させる。
func TestWriteLeavesOriginalIntactOnFailure(t *testing.T) {
	t.Parallel()

	if os.Geteuid() == 0 {
		t.Skip("root は書き込み権限の制限を受けないため、この経路は検証できない")
	}

	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	write(t, path, "A=1\n", 0o600)

	// ディレクトリを読み取り専用にすると一時ファイルの作成が失敗する。
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatalf("パーミッションの変更に失敗: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	if err := fileio.Write(path, []byte("A=2\n"), 0o600); err == nil {
		t.Fatal("Write() = nil, want エラー")
	}

	if got := read(t, path); got != "A=1\n" {
		t.Errorf("失敗後の内容 = %q, want %q（元の内容が壊れている）", got, "A=1\n")
	}
}

// 中途半端な一時ファイルを残さないこと。失敗しても成功しても掃除する。
func TestWriteLeavesNoTempFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, ".env")

	if err := fileio.Write(path, []byte("A=1\n"), 0o600); err != nil {
		t.Fatalf("Write() でエラー: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir() でエラー: %v", err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".env.") {
			t.Errorf("一時ファイルが残っている: %s", e.Name())
		}
	}
}

// シンボリックリンクを書き込みの対象にしないこと。runner ディレクトリは runner
// 実行ユーザーが書き換えられるため、リンクを辿ると root がリンク先を上書きする。
func TestWriteRefusesSymlink(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	victim := filepath.Join(dir, "victim")
	write(t, victim, "守るべき内容\n", 0o600)

	link := filepath.Join(dir, ".env")
	if err := os.Symlink(victim, link); err != nil {
		t.Fatalf("シンボリックリンクの作成に失敗: %v", err)
	}

	err := fileio.Write(link, []byte("A=2\n"), 0o600)
	if !errors.Is(err, fileio.ErrSymlink) {
		t.Fatalf("Write() のエラー = %v, want ErrSymlink", err)
	}
	if got := read(t, victim); got != "守るべき内容\n" {
		t.Errorf("リンク先が書き換えられた: %q", got)
	}
}

// 通常ファイル以外は対象にしないこと。
func TestWriteRefusesDirectory(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0o700); err != nil {
		t.Fatalf("ディレクトリの作成に失敗: %v", err)
	}

	if err := fileio.Write(sub, []byte("x"), 0o600); !errors.Is(err, fileio.ErrNotRegular) {
		t.Fatalf("Write() のエラー = %v, want ErrNotRegular", err)
	}
}

// Read の上限とリンク・不在の扱い。
func TestRead(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	write(t, path, "A=1\n", 0o600)

	got, err := fileio.Read(path, 1024)
	if err != nil || string(got) != "A=1\n" {
		t.Fatalf("Read() = %q, %v, want %q, nil", got, err, "A=1\n")
	}

	if _, err := fileio.Read(path, 2); !errors.Is(err, fileio.ErrTooLarge) {
		t.Errorf("上限超過の Read() のエラー = %v, want ErrTooLarge", err)
	}

	missing := filepath.Join(dir, "missing")
	if _, err := fileio.Read(missing, 1024); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("不在の Read() のエラー = %v, want fs.ErrNotExist", err)
	}

	link := filepath.Join(dir, "link")
	if err := os.Symlink(path, link); err != nil {
		t.Fatalf("シンボリックリンクの作成に失敗: %v", err)
	}
	if _, err := fileio.Read(link, 1024); !errors.Is(err, fileio.ErrSymlink) {
		t.Errorf("リンクの Read() のエラー = %v, want ErrSymlink", err)
	}
}

// 新規作成のファイルが親ディレクトリの所有者を引き継ぐこと。
//
// **root で動く本ツールが runner 所有のディレクトリに root:root の .env を作る
// 事故を防ぐ回帰テストである。** .env が無いのは普通の状態なので、新規作成で
// chown を省くと runner が自分の設定を読めなくなる。
func TestWriteNewFileInheritsDirOwner(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, ".env")

	if err := fileio.Write(path, []byte("A=1\n"), 0o600); err != nil {
		t.Fatalf("Write() でエラー: %v", err)
	}

	fi, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat() でエラー: %v", err)
	}
	if got := fi.Mode().Perm(); got != 0o600 {
		t.Errorf("パーミッション = %o, want 600", got)
	}

	di, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("親ディレクトリの Stat() でエラー: %v", err)
	}
	fst, fok := fi.Sys().(*syscall.Stat_t)
	dst, dok := di.Sys().(*syscall.Stat_t)
	if !fok || !dok {
		t.Skip("uid/gid を取り出せない環境では検証できない")
	}
	if fst.Uid != dst.Uid || fst.Gid != dst.Gid {
		t.Errorf("所有者 = %d:%d, want 親ディレクトリと同じ %d:%d", fst.Uid, fst.Gid, dst.Uid, dst.Gid)
	}
}

// 所有者が違う親ディレクトリでも、新規作成のファイルがその所有者になること。
//
// chown(2) は root でないと他人の uid へ変えられないため、非 root では飛ばす。
func TestWriteNewFileChownsToDirOwner(t *testing.T) {
	t.Parallel()

	if os.Geteuid() != 0 {
		t.Skip("root でないと他の所有者への chown を検証できない")
	}

	dir := t.TempDir()
	const uid, gid = 65534, 65534 // nobody:nogroup
	if err := os.Chown(dir, uid, gid); err != nil {
		t.Skipf("親ディレクトリの chown に失敗（uid %d が無い環境）: %v", uid, err)
	}

	path := filepath.Join(dir, ".env")
	if err := fileio.Write(path, []byte("A=1\n"), 0o600); err != nil {
		t.Fatalf("Write() でエラー: %v", err)
	}

	fi, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat() でエラー: %v", err)
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		t.Skip("uid/gid を取り出せない環境では検証できない")
	}
	if int(st.Uid) != uid || int(st.Gid) != gid {
		t.Errorf("所有者 = %d:%d, want %d:%d", st.Uid, st.Gid, uid, gid)
	}
}
