package logs

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// errNotRegular は開いた先が通常ファイルでなかった場合のエラー。
var errNotRegular = errors.New("通常ファイルではありません")

// openRegular は path を読み込み用に開き、通常ファイルであることを確かめて返す。
//
// `_diag` は runner 実行ユーザー（ジョブが任意のコードを走らせる）が書き換えられる
// ので、`Runner_x.log` の名前で FIFO やデバイスへのシンボリックリンクを置かれうる。
// os.Open のままだと FIFO は書き手が現れるまで open(2) が返らず、/dev/zero は
// 改行の来ない無限の入力になる。そこで config/fileio の読み込みと同じ流儀で守る。
//
//   - O_NOFOLLOW: 終端がシンボリックリンクなら open(2) 自身が ELOOP を返す。
//     Lstat で確かめてから開く方法は確認と open の間に差し替えられる（TOCTOU）。
//   - O_NONBLOCK: O_NOFOLLOW は FIFO を防げない。通常ファイルかの判定は open の
//     後にしかできないので、open 自体が返らない経路をここで塞ぐ。
//   - Stat による通常ファイルの確認: デバイスやソケットを読みに行かない。
//
// logs が config/fileio を使わないのは、依存の向き（docs/components/overview.md）を
// 増やさないためである。必要なのはこの 10 行ほどで、書き込み側の安全策は要らない。
func openRegular(path string) (*os.File, error) {
	flags := syscall.O_RDONLY | syscall.O_NOFOLLOW | syscall.O_CLOEXEC | syscall.O_NONBLOCK

	fd, err := syscall.Open(filepath.Clean(path), flags, 0)
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: path, Err: err}
	}
	f := os.NewFile(uintptr(fd), path)
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("%s の情報を取得できません: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		_ = f.Close()
		return nil, fmt.Errorf("%s: %w", path, errNotRegular)
	}
	return f, nil
}
