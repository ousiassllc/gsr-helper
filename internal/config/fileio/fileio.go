// Package fileio は runner ディレクトリ配下のファイルを安全に読み書きする。
//
// internal/config の下位パッケージに分けているのは、行数上限（1 ファイル 300 行 /
// 1 ディレクトリ 2000 行）のためと、「どう読み書きするか」が「何を読み書きするか」
// （.env / .path / systemd drop-in）とは独立した責務だからである。読み書きの安全策を
// 1 か所に集めておけば、対象が増えても守り方が分岐しない。
//
// ここが守るのは 3 つである。
//
//   - シンボリックリンクを対象にしない。runner ディレクトリは runner 実行ユーザーが
//     書き換えられる場所であり、.env が /etc/shadow へのリンクにすり替えられていた
//     場合、root で動く本ツールがその先を上書きしてしまう。
//   - 書き込みは一時ファイル + rename で行う。途中で失敗しても、中途半端な内容の
//     ファイルが元の位置に残らない。
//   - 既存ファイルの所有者とパーミッションを引き継ぐ。root で書き戻した結果 runner
//     自身が .env を読めなくなる、という事故を防ぐ（docs/architecture/security.md）。
package fileio

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
)

// 読み書きのエラー。呼び出し側は errors.Is で判定する。
//
// 対象が存在しない場合は fs.ErrNotExist を包んだエラーを返す。専用の番兵を
// 足さないのは、errors.Is(err, fs.ErrNotExist) が標準の判定手段であり、
// 別名を作ると判定方法が 2 つになるだけだからである。
var (
	// ErrSymlink は対象がシンボリックリンクだった場合のエラー。
	ErrSymlink = errors.New("シンボリックリンクは読み書きの対象にできません")
	// ErrNotRegular は対象が通常ファイルでない場合のエラー。
	ErrNotRegular = errors.New("通常ファイルではありません")
	// ErrTooLarge は読み込みの上限を超えた場合のエラー。
	ErrTooLarge = errors.New("ファイルが大きすぎます")
)

// Read は path を最大 limit バイトまで読む。
//
// シンボリックリンクは辿らずに ErrSymlink を返す。通常ファイルでなければ
// ErrNotRegular を、limit を超えていれば ErrTooLarge を返す。
func Read(path string, limit int64) ([]byte, error) {
	b, _, err := read(path, limit)
	return b, err
}

// read は Read の実体で、読み取った内容と開いたファイル自身の情報を返す。
//
// 情報を fd から取るのは、path をもう一度 Lstat すると「読んだファイル」と
// 「情報を見たファイル」が別物になりうるためである（Backup が所有者と
// パーミッションを引き継ぐときにこれが要る）。
func read(path string, limit int64) ([]byte, fs.FileInfo, error) {
	f, err := openNoFollow(path)
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = f.Close() }()

	fi, err := f.Stat()
	if err != nil {
		return nil, nil, fmt.Errorf("%s の状態の取得に失敗しました: %w", path, err)
	}
	if !fi.Mode().IsRegular() {
		return nil, nil, fmt.Errorf("%s: %w", path, ErrNotRegular)
	}

	b, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, nil, fmt.Errorf("%s の読み込みに失敗しました: %w", path, err)
	}
	if int64(len(b)) > limit {
		return nil, nil, fmt.Errorf("%s: %w（上限 %d バイト）", path, ErrTooLarge, limit)
	}
	return b, fi, nil
}

// openNoFollow は O_NOFOLLOW 付きで path を読み込み用に開く。
//
// 終端がシンボリックリンクなら open(2) 自身が ELOOP を返すため、リンクを辿る前に
// 弾ける。Lstat で確かめてから開く方法だと、確認と open の間に差し替えられる
// （TOCTOU）。
//
// **O_NONBLOCK を必ず付ける。** O_NOFOLLOW が防ぐのはシンボリックリンクだけで、
// 名前付きパイプ（FIFO）は防げない。runner ディレクトリは runner 実行ユーザーが
// 書き換えられるので、.env を FIFO にすり替えられると open(2) が書き手の現れる
// まで返らない。読み取りは 3 秒ごとの状態更新から同期的に呼ばれるため、TUI 全体が
// 復帰不能に固まる。通常ファイルかどうかの判定は open の後にしかできない以上、
// open 自体が返らない経路を塞ぐ必要がある。
func openNoFollow(path string) (*os.File, error) {
	flags := syscall.O_RDONLY | syscall.O_NOFOLLOW | syscall.O_CLOEXEC | syscall.O_NONBLOCK

	fd, err := syscall.Open(filepath.Clean(path), flags, 0)
	if err != nil {
		if errors.Is(err, syscall.ELOOP) {
			return nil, fmt.Errorf("%s: %w", path, ErrSymlink)
		}
		return nil, fmt.Errorf("%s を開けませんでした: %w", path, err)
	}
	return os.NewFile(uintptr(fd), path), nil
}
