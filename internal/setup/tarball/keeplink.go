package tarball

import (
	"archive/tar"
	"fmt"
	"path"
	"path/filepath"
	"strings"
)

// linkSet はこの展開で作ったリンクの向き先を、展開先からの相対パスで覚える。
//
// tar は `link -> .runner` というリンクの直後に `link/pwned` を並べられる。
// 名前の先頭要素しか見ない isPreserved はこれを保持対象と判定できず、書き込みは
// リンクを辿って .runner の中へ落ちる。os.Root が守るのは展開先の外への逸脱だけ
// なので、FR-21 の保持はここで別途守る必要がある。作ったリンクを覚えておき、
// 見かけの名前ではなく実際に書き込まれる場所で保持判定を行う。
type linkSet map[string]string

// resolve は name を、これまでに作ったリンクを辿った先の相対パスへ直す。
//
// 先頭から 1 要素ずつ辿るのは、途中の階層がリンクでも取りこぼさないためである。
func (ls linkSet) resolve(name string) string {
	if len(ls) == 0 {
		return name
	}

	cur := ""
	for _, elem := range strings.Split(name, "/") {
		cur = path.Join(cur, elem)
		if to, ok := ls[cur]; ok {
			cur = to
		}
	}
	return cur
}

// remember はエントリがリンクなら、その向き先を覚える。リンク以外は何もしない。
func (ls linkSet) remember(hdr *tar.Header, name string) {
	if target, ok := ls.linkTarget(hdr, name); ok {
		ls[ls.resolve(name)] = target
	}
}

// linkTarget はリンクの向き先を展開先からの相対パスで返す。リンク以外は ok が偽。
//
// シンボリックリンクの向き先はリンク自身の親からの相対、ハードリンクの向き先は
// アーカイブの根からの相対と、基準が違う。どちらも展開先からの相対へ揃えてから
// 保持判定に掛ける。
func (ls linkSet) linkTarget(hdr *tar.Header, name string) (string, bool) {
	link := filepath.ToSlash(hdr.Linkname)

	switch hdr.Typeflag {
	case tar.TypeSymlink:
		return ls.resolve(path.Join(path.Dir(ls.resolve(name)), link)), true
	case tar.TypeLink:
		return ls.resolve(path.Clean(link)), true
	default:
		return "", false
	}
}

// checkKeep はリンクが保持対象へ潜り込んでいないかを確かめる（FR-21）。
//
// 読み飛ばさずエラーにするのは、正規の runner tarball が保持対象を指すリンクを
// 一切含まないからである。含まれていれば tarball が細工されているということで、
// 黙って続きを展開してよい状況ではない。
func (ls linkSet) checkKeep(hdr *tar.Header, name string, keep []string) error {
	target, ok := ls.linkTarget(hdr, name)
	if !ok {
		return nil
	}
	if isPreserved(ls.resolve(name), keep) || isPreserved(target, keep) {
		return fmt.Errorf("%w: %s -> %s", ErrPreservedLink, hdr.Name, hdr.Linkname)
	}
	return nil
}
