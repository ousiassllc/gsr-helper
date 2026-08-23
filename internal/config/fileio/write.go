package fileio

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
)

// Write は path の内容を b で置き換える。
//
// 既存ファイルがあれば所有者（uid/gid）とパーミッションを引き継ぎ、無ければ
// newMode で作る。書き込みは同じディレクトリの一時ファイルへ行い rename で
// 差し替えるため、途中で失敗しても元のファイルはそのまま残る。
//
// path がシンボリックリンクなら ErrSymlink を、通常ファイル以外なら
// ErrNotRegular を返す。rename 自体はリンクを辿らず置き換えるので、判定と
// 置き換えの間にすり替えられてもリンクの先へは書き込まれない。
func Write(path string, b []byte, newMode fs.FileMode) error {
	path = filepath.Clean(path)

	mode, own, err := target(path, newMode)
	if err != nil {
		return err
	}
	return writeAs(path, b, mode, own)
}

// writeAs は所有者とパーミッションを指定して path を置き換える。
//
// 一時ファイルを同じディレクトリに作るのは、ファイルシステムをまたぐ rename が
// 失敗するためである。名前をドットで始めるのは、残骸が runner の設定ファイルとして
// 拾われないようにするためである。
func writeAs(path string, b []byte, mode fs.FileMode, own owner) error {
	dir := filepath.Dir(path)

	f, err := os.CreateTemp(dir, "."+filepath.Base(path)+".*")
	if err != nil {
		return fmt.Errorf("%s への一時ファイルの作成に失敗しました: %w", dir, err)
	}
	tmp := f.Name()
	defer func() { _ = os.Remove(tmp) }()

	if err := fill(f, b, mode, own); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("%s への書き込みに失敗しました: %w", path, err)
	}
	return nil
}

// owner は引き継ぐ所有者。ok が false なら chown しない（新規作成の場合）。
type owner struct {
	uid int
	gid int
	ok  bool
}

// target は書き込み先の現状から、引き継ぐパーミッションと所有者を決める。
//
// **まだファイルが無い場合は親ディレクトリの所有者を引き継ぐ。** runner の .env が
// 無いのは普通の状態であり（envfile.Load の doc）、chown を省くと sudo 実行時に
// runner 所有のディレクトリの中へ root:root のファイルができて runner が自分の
// 設定を読めなくなる——このパッケージが防ぐと言っている事故そのものである。
func target(path string, newMode fs.FileMode) (fs.FileMode, owner, error) {
	fi, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return newMode.Perm(), dirOwner(filepath.Dir(path)), nil
	}
	if err != nil {
		return 0, owner{uid: 0, gid: 0, ok: false}, fmt.Errorf("%s の状態の取得に失敗しました: %w", path, err)
	}
	if fi.Mode()&fs.ModeSymlink != 0 {
		return 0, owner{uid: 0, gid: 0, ok: false}, fmt.Errorf("%s: %w", path, ErrSymlink)
	}
	if !fi.Mode().IsRegular() {
		return 0, owner{uid: 0, gid: 0, ok: false}, fmt.Errorf("%s: %w", path, ErrNotRegular)
	}
	return fi.Mode().Perm(), ownerOf(fi), nil
}

// dirOwner は親ディレクトリの所有者を返す。読めなければ chown しない。
//
// 読めない場合に諦めるのは、ここで失敗させると「所有者を合わせられないから
// 書けない」という止め方になり、書き込み自体は成功しうる場面を潰すためである。
func dirOwner(dir string) owner {
	fi, err := os.Stat(dir)
	if err != nil {
		return owner{uid: 0, gid: 0, ok: false}
	}
	return ownerOf(fi)
}

// ownerOf は FileInfo から uid/gid を取り出す。取り出せない環境では ok が false になる。
func ownerOf(fi fs.FileInfo) owner {
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return owner{uid: 0, gid: 0, ok: false}
	}
	return owner{uid: int(st.Uid), gid: int(st.Gid), ok: true}
}

// fill は一時ファイルを目的の状態にして閉じる。
func fill(f *os.File, b []byte, mode fs.FileMode, own owner) error {
	err := prepare(f, b, mode, own)
	// Close の戻り値も見る。書き込みの失敗は Close ではじめて現れることがある。
	if cerr := f.Close(); err == nil && cerr != nil {
		err = fmt.Errorf("一時ファイルのクローズに失敗しました: %w", cerr)
	}
	return err
}

// prepare は open 済みの一時ファイルにパーミッション・所有者・内容を設定する。
func prepare(f *os.File, b []byte, mode fs.FileMode, own owner) error {
	// os.CreateTemp は 0600 で作り、さらに umask の影響も受けるため明示的に合わせる。
	if err := f.Chmod(mode); err != nil {
		return fmt.Errorf("一時ファイルのパーミッション設定に失敗しました: %w", err)
	}
	if err := chown(f, own); err != nil {
		return err
	}
	if _, err := f.Write(b); err != nil {
		return fmt.Errorf("一時ファイルへの書き込みに失敗しました: %w", err)
	}
	if err := f.Sync(); err != nil {
		return fmt.Errorf("一時ファイルの同期に失敗しました: %w", err)
	}
	return nil
}

// chown は必要なときだけ所有者を合わせる。
//
// 既に同じ所有者なら呼ばないのは、非 root の chown(2) が EPERM になるためである。
// 自分のファイルを自分で書き戻すという普通の場合を、無意味な chown で失敗させない。
func chown(f *os.File, own owner) error {
	if !own.ok {
		return nil
	}
	fi, err := f.Stat()
	if err != nil {
		return fmt.Errorf("一時ファイルの状態の取得に失敗しました: %w", err)
	}
	if cur := ownerOf(fi); cur.ok && cur.uid == own.uid && cur.gid == own.gid {
		return nil
	}
	if err := f.Chown(own.uid, own.gid); err != nil {
		return fmt.Errorf("一時ファイルの所有者変更に失敗しました: %w", err)
	}
	return nil
}
