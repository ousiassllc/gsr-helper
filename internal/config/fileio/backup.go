package fileio

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

const (
	// BackupSuffix はバックアップファイルに付ける拡張子。
	// 画面仕様の例（/opt/runners/build01-1/.env.bak）に合わせる。
	BackupSuffix = ".bak"

	// MaxBackupSize は Backup が扱うファイルサイズの上限。
	// 対象は .env / .path / drop-in のいずれも数 KB のファイルであり、
	// これを大きく超える入力はバックアップではなく調査の対象である。
	MaxBackupSize int64 = 8 << 20
)

// Backup は path のバックアップを <path>.bak として作る（FR-38）。
//
// 元ファイルの所有者とパーミッションを引き継ぐ。バックアップだけが root 所有の
// 0600 で残ると、利用者が自分で戻せなくなるためである。
//
// path が無い場合は fs.ErrNotExist を包んだエラーを返すので、呼び出し側は
// errors.Is(err, fs.ErrNotExist) で「まだ元ファイルが無いだけ」を判別できる。
// path とバックアップ先のどちらかがシンボリックリンクなら ErrSymlink を返す。
//
// 既存の <path>.bak は置き換える。世代を持たないのは、書き込み前の 1 つ前へ
// 戻せれば足りるという FR-38 の目的に対して、世代管理は runner ディレクトリを
// 太らせるだけだからである。置き換えは一時ファイル + rename で行うため、
// 途中で失敗しても前回のバックアップが壊れた状態で残ることはない。
func Backup(path string) error {
	path = filepath.Clean(path)

	b, fi, err := read(path, MaxBackupSize)
	if err != nil {
		return err
	}

	dst := path + BackupSuffix
	if err := ensureReplaceable(dst); err != nil {
		return err
	}
	return writeAs(dst, b, fi.Mode().Perm(), ownerOf(fi))
}

// ensureReplaceable は dst が rename で置き換えてよい対象かを確かめる。
//
// rename はシンボリックリンクを辿らずリンク自体を置き換えるため、これを
// 通さなくてもリンクの先へ書き込まれることはない。それでも弾くのは、
// 「.env.bak がリンクだった」という状態を黙って消してしまわないためである。
func ensureReplaceable(dst string) error {
	fi, err := os.Lstat(dst)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("%s の状態の取得に失敗しました: %w", dst, err)
	}
	if fi.Mode()&fs.ModeSymlink != 0 {
		return fmt.Errorf("%s: %w", dst, ErrSymlink)
	}
	if !fi.Mode().IsRegular() {
		return fmt.Errorf("%s: %w", dst, ErrNotRegular)
	}
	return nil
}
