// このファイルは配置先への副作用（作成・所有者・パーミッション）だけを持つ。
// 「どこに置くか」を決める純粋な処理（confpath.go）と分けているのは、root 権限で
// ファイルシステムを変える箇所を 1 ファイルに閉じ、レビューの対象を狭めるためである。

package confpath

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"syscall"
)

// security.md の表のとおり、設定ファイルは 600、ディレクトリは 700。
const (
	// DirMode は設定ディレクトリのパーミッション。
	DirMode os.FileMode = 0o700
	// FileMode は設定ファイルのパーミッション。
	FileMode os.FileMode = 0o600
)

// fsOps は所有者・パーミッションを変える副作用。テストで差し替えるため関数値で持つ。
type fsOps struct {
	geteuid func() int
	chmod   func(string, os.FileMode) error
	lchown  func(string, int, int) error
}

// realFS は実環境の副作用。
//
// chown ではなく lchown を使う。chown(2) はリンクを辿るため、missingDirs の Stat →
// MkdirAll → 所有者変更の隙に対象ユーザーが <home>/.config/gsr-helper を任意パスへの
// シンボリックリンクへ差し替えると、root がそのリンク先を当該ユーザー所有に変えて
// しまう。lchown はリンク自体を対象にするので無害化される（internal/audit が
// O_NOFOLLOW で断っているのと同じ脅威に、同じ方針で対処する）。
//
// chmod も同じ理由で os.Chmod を使わない。chmod(2) もリンクを辿るため、同じ隙に
// リンクを差し替えられると root が任意のファイルのモードを 0700 に変えてしまう
// （世界から読めていたファイルを読めなくする）。chmodNoFollow が O_NOFOLLOW で
// 開いた fd に対して fchmod するので、リンクなら開く時点で失敗する。
var realFS = fsOps{geteuid: os.Geteuid, chmod: chmodNoFollow, lchown: os.Lchown}

// chmodNoFollow は dir のモードを変える。dir がシンボリックリンクなら失敗する。
//
// O_NOFOLLOW | O_DIRECTORY で開いた fd に対して fchmod するため、開いた実体と
// モードを変える実体が同一であることが保証される（パス名で 2 回参照しないので
// 途中で差し替えられない）。
func chmodNoFollow(dir string, mode os.FileMode) error {
	//nolint:gosec // dir は本ツールが決めた設定ディレクトリ（<home>/.config/gsr-helper）または
	// --config で指されたその親であり、開くこと自体が本関数の目的。O_NOFOLLOW でリンクは断つ。
	f, err := os.OpenFile(dir, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_DIRECTORY, 0)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	return f.Chmod(mode)
}

// MkdirOwned は dir を 700 で作り、本ツールが責任を持つディレクトリだけを所有者に合わせる。
//
// MkdirAll の前に「存在しない祖先」を列挙し、その中でも対象ユーザーのホームより
// 下にあるものだけを chown 対象にする。こうしておくと、既存のディレクトリや
// /home のような共有ディレクトリを chown する事故が構造的に起こらない。
func (o Owner) MkdirOwned(dir string) error {
	return o.mkdirOwnedFS(dir, realFS)
}

// mkdirOwnedFS は副作用を差し替えられる本体。
func (o Owner) mkdirOwnedFS(dir string, ops fsOps) error {
	// ホームの作成は本ツールの責務ではない。root で作ると root 所有 0700 になり、
	// 対象ユーザーが自分のホームを通れず設定を読めなくなる（security.md 4.）。
	if fi, err := os.Stat(o.home); err != nil || !fi.IsDir() {
		return fmt.Errorf("%s のホームディレクトリ %s がありません", o.name, o.home)
	}

	targets := missingDirs(dir, o.home)
	if err := os.MkdirAll(dir, DirMode); err != nil {
		return fmt.Errorf("%s の作成に失敗しました: %w", dir, err)
	}

	// leaf は本ツール専用のディレクトリなので、既存でも mode と所有者を締め直す。
	// root が 0755 で作ったまま残っていると、次に非 root で起動したときに一時
	// ファイルを作れず自分の設定を書けない（security.md 4.）。
	// ~/.config のような共有の祖先と、--config で指された任意のディレクトリは
	// 他の用途と共有されうるので触らない。
	//
	// 線引きはディレクトリ名と「対象ユーザーのホーム配下であること」の両方で行う。
	// 名前だけで判断すると、--config /etc/gsr-helper/config.yaml のような指定で
	// root が /etc/gsr-helper を 0700 にし、さらに非特権ユーザー所有へ chown して
	// しまう。本ツールへの sudo だけを許されたユーザーにとっては権限昇格になる。
	if filepath.Base(dir) == DirName && underHome(o.home, dir) {
		if err := ops.chmod(dir, DirMode); err != nil {
			return fmt.Errorf("%s のパーミッション設定に失敗しました: %w", dir, err)
		}
		if !slices.Contains(targets, dir) {
			targets = append(targets, dir)
		}
	}

	uid, gid, ok := o.ChownTarget(ops.geteuid())
	if !ok {
		return nil
	}
	return chownDirs(targets, uid, gid, ops.lchown)
}

// chownDirs は dirs の所有者を uid/gid に合わせる。
// リンクを辿らない理由は realFS のコメントを参照。
func chownDirs(dirs []string, uid, gid int, lchown func(string, int, int) error) error {
	for _, d := range dirs {
		if err := lchown(d, uid, gid); err != nil {
			return fmt.Errorf("%s の所有者変更に失敗しました: %w", d, err)
		}
	}
	return nil
}

// missingDirs は dir までの経路のうち、まだ存在せず home より下にあるものを
// 浅い順に返す。home 自身とそれより上は含めない。
func missingDirs(dir, home string) []string {
	var missing []string
	h := filepath.Clean(home)
	for p := filepath.Clean(dir); p != h && underHome(h, p); p = filepath.Dir(p) {
		if _, err := os.Stat(p); err == nil {
			break
		}
		missing = append(missing, p)
	}
	slices.Reverse(missing)
	return missing
}

// ChownTarget は chown すべき uid/gid を返す。ok が false なら chown しない。
//
// 実効 UID が 0 のときだけ、かつ対象が root 以外のときだけ chown する。
// 非 root では必ず失敗する呼び出しになるため判定を純粋関数に切り出しており、
// 非 root で動く CI でもこの分岐をテストできる。
func (o Owner) ChownTarget(euid int) (int, int, bool) {
	switch {
	case euid != 0:
		return 0, 0, false
	case o.uid == 0 && o.gid == 0:
		return 0, 0, false
	default:
		return o.uid, o.gid, true
	}
}
