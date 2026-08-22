// このファイルは配置先への副作用（作成・所有者・パーミッション）だけを持つ。
// 「どこに置くか」を決める純粋な処理（confpath.go）と分けているのは、root 権限で
// ファイルシステムを変える箇所を 1 ファイルに閉じ、レビューの対象を狭めるためである。

package confpath

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
)

// security.md の表のとおり、設定ファイルは 600、ディレクトリは 700。
const (
	// DirMode は設定ディレクトリのパーミッション。
	DirMode os.FileMode = 0o700
	// FileMode は設定ファイルのパーミッション。
	FileMode os.FileMode = 0o600
)

// fsOps は所有者・パーミッションを変える副作用。テストで差し替えるため関数値で持つ。
//
// パスは *os.Root（対象ユーザーのホーム）からの相対で受ける。絶対パスで受けると
// 「どのホームに閉じた操作なのか」が呼び出しの形に現れず、ホームの外を触る
// 呼び出しを書けてしまう。
type fsOps struct {
	geteuid func() int
	chmod   func(root *os.Root, name string, mode os.FileMode) error
	lchown  func(root *os.Root, name string, uid, gid int) error
}

// realFS は実環境の副作用。*os.Root のメソッドをそのまま使う。
//
// os.Chmod / os.Chown を使わない。どちらもリンクを辿るため、「存在しない祖先の
// 列挙 → MkdirAll → 所有者変更」の隙に対象ユーザーが <home>/.config を任意パスへの
// シンボリックリンクへ差し替えると、root がそのリンク先を 0700 にし、当該ユーザー
// 所有へ変えてしまう（internal/audit が O_NOFOLLOW で断っているのと同じ脅威）。
//
// リンク自体を対象にする lchown や、O_NOFOLLOW で開いた fd への fchmod でも足りない。
// どちらも守るのは最後の要素だけで、経路の途中の要素がリンクに差し替えられた場合は
// 通ってしまう。underHome の判定はパス名の字句だけを見るため、この差し替えを
// 「ホーム配下のパス」と読んでしまう。
//
// *os.Root はホーム（根）の外へ出る参照を各要素の解決時に拒む。相対リンクで
// ホーム内に留まる場合だけ辿り、絶対リンクとホーム外へ出るリンク、.. による
// 離脱はエラーになる。字句判定ではなく実際の解決で閉じ込めるので、途中の要素を
// 差し替えられてもホームの外を触ることがない。
var realFS = fsOps{geteuid: os.Geteuid, chmod: (*os.Root).Chmod, lchown: (*os.Root).Lchown}

// MkdirOwned は dir を 700 で作り、本ツールが責任を持つディレクトリだけを所有者に合わせる。
//
// 対象ユーザーのホームの外（--config で任意の場所を指した場合）は作るだけで、
// mode も所有者も変えない。ホームの中は os.Root 経由で操作し、経路がホームの外へ
// 出る場合はエラーにして何もしない。
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

	dir = filepath.Clean(dir)
	if !underHome(o.home, dir) {
		// --config で指された任意のディレクトリは他の用途と共有されうるので触らない。
		// 名前だけで leaf と判断すると、--config /etc/gsr-helper/config.yaml のような
		// 指定で root が /etc/gsr-helper を 0700 にし、さらに非特権ユーザー所有へ
		// chown してしまう。本ツールへの sudo だけを許されたユーザーにとっては
		// 権限昇格になる。
		if err := os.MkdirAll(dir, DirMode); err != nil {
			return fmt.Errorf("%s の作成に失敗しました: %w", dir, err)
		}
		return nil
	}
	return o.mkdirInHome(dir, ops)
}

// mkdirInHome は対象ユーザーのホーム配下の dir を作り、締め直す。
//
// ホームを根とする os.Root の中だけで操作するため、経路の途中の要素がホーム外への
// シンボリックリンクに差し替えられていた場合は MkdirAll の時点で失敗し、
// リンクの先には何も作らず、mode も所有者も変えない。
func (o Owner) mkdirInHome(dir string, ops fsOps) error {
	rel, err := filepath.Rel(o.home, dir)
	if err != nil {
		return fmt.Errorf("%s の作成に失敗しました: %w", dir, err)
	}
	root, err := os.OpenRoot(o.home)
	if err != nil {
		return fmt.Errorf("%s を開けませんでした: %w", o.home, err)
	}
	defer func() { _ = root.Close() }()

	// MkdirAll の前に「存在しない祖先」を列挙し、それだけを chown 対象にする。
	// こうしておくと、既存のディレクトリを chown する事故が構造的に起こらない。
	targets := missingDirs(root, rel)
	if err := root.MkdirAll(rel, DirMode); err != nil {
		return fmt.Errorf("%s の作成に失敗しました: %w", dir, err)
	}

	// leaf は本ツール専用のディレクトリなので、既存でも mode と所有者を締め直す。
	// root が 0755 で作ったまま残っていると、次に非 root で起動したときに一時
	// ファイルを作れず自分の設定を書けない（security.md 4.）。
	// ~/.config のような共有の祖先は他の用途と共有されうるので触らない。
	if filepath.Base(rel) == DirName {
		if err := chmodLeaf(root, rel, ops.chmod); err != nil {
			return fmt.Errorf("%s のパーミッション設定に失敗しました: %w", dir, err)
		}
		if !slices.Contains(targets, rel) {
			targets = append(targets, rel)
		}
	}

	uid, gid, ok := o.ChownTarget(ops.geteuid())
	if !ok {
		return nil
	}
	return chownDirs(root, targets, uid, gid, ops.lchown)
}

// chmodLeaf は leaf の mode を DirMode にする。leaf がリンクなら締め直さない。
//
// os.Root はホームの外へ出る参照は拒むが、ホーム内で完結する相対リンクは辿る。
// leaf は本ツール専用のディレクトリなので、リンクやファイルに差し替えられていたら
// 意図した実体とは別のものを 0700 にすることになる。締め直しではなく失敗として扱う。
func chmodLeaf(root *os.Root, rel string, chmod func(*os.Root, string, os.FileMode) error) error {
	fi, err := root.Lstat(rel)
	if err != nil {
		return err
	}
	if !fi.IsDir() {
		return errors.New("ディレクトリではありません")
	}
	return chmod(root, rel, DirMode)
}

// chownDirs は dirs（ホームからの相対パス）の所有者を uid/gid に合わせる。
// リンクを辿らない理由は realFS のコメントを参照。
func chownDirs(root *os.Root, dirs []string, uid, gid int,
	lchown func(*os.Root, string, int, int) error,
) error {
	for _, d := range dirs {
		if err := lchown(root, d, uid, gid); err != nil {
			return fmt.Errorf("%s の所有者変更に失敗しました: %w", filepath.Join(root.Name(), d), err)
		}
	}
	return nil
}

// missingDirs は rel までの経路のうち、まだ存在しないものを浅い順に返す。
// 根（ホーム）自身は含めない。
//
// 存在確認は root.Lstat で行う。os.Stat はリンクを辿るため、<home>/.config が
// ホーム外へのリンクに差し替えられていると、リンクの先を見て「既にある」と
// 誤判定する。Lstat ならリンクそのものを見るので、その先の実体を本ツールが
// 作ったものとして扱ってしまうことがない。
func missingDirs(root *os.Root, rel string) []string {
	var missing []string
	for p := rel; p != "."; p = filepath.Dir(p) {
		if _, err := root.Lstat(p); err == nil {
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
