package tarball

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
)

// makeParent はエントリの親ディレクトリを補う。
func makeParent(root *os.Root, name string) error {
	dir := path.Dir(name)
	if dir == "." || dir == "/" {
		return nil
	}
	if err := root.MkdirAll(dir, implicitDirMode); err != nil {
		return fmt.Errorf("ディレクトリ %s の作成に失敗しました: %w", dir, err)
	}
	return nil
}

// checkLinkTarget はリンクの向き先が展開先の中に収まるかを検証する。
func checkLinkTarget(name, linkname string) error {
	target := filepath.ToSlash(linkname)
	if target == "" {
		return fmt.Errorf("%w: %s のリンク先が空です", ErrUnsafePath, name)
	}
	if strings.HasPrefix(target, "/") || filepath.IsAbs(linkname) {
		return fmt.Errorf("%w: %s のリンク先が絶対パスです: %s", ErrUnsafePath, name, linkname)
	}
	if escapes(path.Join(path.Dir(name), target)) {
		return fmt.Errorf("%w: %s のリンク先が展開先の外です: %s", ErrUnsafePath, name, linkname)
	}
	return nil
}

// safeName は tar のエントリ名を展開先からの相対パスへ正規化する（zip-slip 対策）。
//
// アーカイブの根そのものを指す "." / "./" は空文字を返し、呼び出し側が読み飛ばす。
func safeName(name string) (string, error) {
	slashed := filepath.ToSlash(name)
	if strings.HasPrefix(slashed, "/") || filepath.IsAbs(name) {
		return "", fmt.Errorf("%w: 絶対パスのエントリです: %s", ErrUnsafePath, name)
	}

	// Clean する前に判定する。Clean は "a/../b" を "b" に畳んでしまい、逸脱を
	// 試みたエントリだったことを検出できなくなる。
	for _, elem := range strings.Split(slashed, "/") {
		if elem == ".." {
			return "", fmt.Errorf(`%w: ".." を含むエントリです: %s`, ErrUnsafePath, name)
		}
	}

	// 上の走査で ".." を落としてあるため、Clean の結果が展開先の外を指すことは無い。
	// 万一すり抜けても、書き込みは os.Root の内側に閉じているので外へは出られない。
	cleaned := path.Clean(slashed)
	if cleaned == "." || cleaned == "/" {
		return "", nil
	}
	return cleaned, nil
}

// escapes は Clean 済みの相対パスが展開先の外を指しているかを返す。
func escapes(rel string) bool {
	return rel == ".." || strings.HasPrefix(rel, "../")
}

// isPreserved は name の先頭要素が keep に載っているかを返す（FR-21）。
func isPreserved(name string, keep []string) bool {
	top, _, _ := strings.Cut(name, "/")
	return slices.Contains(keep, top)
}
