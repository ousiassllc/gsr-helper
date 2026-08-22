package disk

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

// allowedSubtrees は削除を許可するサブツリー。runner ディレクトリ直下のこの 2 つと
// その配下だけを削除できる（docs/architecture/security.md「削除パスの検証を必須にする」）。
// bin / externals / .runner などツールの動作に必要なものを巻き込まないためである。
var allowedSubtrees = []string{"_work", "_diag"}

// ValidatePath は target が base 配下の削除してよいパスかを検証する。
//
// root 権限で動くため、削除を行う関数はこの検証を通らない限り実行できない構造に
// してある（PlanClean と Apply の両方から呼ぶ）。エラー文はどの条件で落ちたかが
// 読み分けられるようにしてあり、利用者への表示と異常系テストの両方で使う。
//
// 検証はシンボリックリンクを解決してから行う。解決しないと _work 内のリンク経由で
// 基準ディレクトリの外へ抜けられる。逆に削除時（removeTree）はリンクを辿らない。
func ValidatePath(base, target string) error {
	if base == "" {
		return errors.New("基準ディレクトリが空です")
	}
	if target == "" {
		return errors.New("削除対象のパスが空です")
	}

	// Clean する前に判定する。filepath.Clean は "a/../b" を "b" に畳んでしまい、
	// 呼び出し側が意図せず相対参照を渡したことを検出できなくなる。
	if hasDotDot(target) {
		return fmt.Errorf(`削除対象のパスに ".." が含まれています: %s`, target)
	}

	absBase, err := filepath.Abs(base)
	if err != nil {
		return fmt.Errorf("基準ディレクトリの絶対パス化に失敗しました: %s: %w", base, err)
	}
	absTarget, err := filepath.Abs(target)
	if err != nil {
		return fmt.Errorf("削除対象のパスの絶対パス化に失敗しました: %s: %w", target, err)
	}

	// 存在しないパスは EvalSymlinks が失敗する。存在しないものを削除対象にする
	// 必要は無いので、失敗はそのまま拒否として扱う。
	realBase, err := filepath.EvalSymlinks(absBase)
	if err != nil {
		return fmt.Errorf("基準ディレクトリのシンボリックリンク解決に失敗しました: %s: %w", base, err)
	}
	realTarget, err := filepath.EvalSymlinks(absTarget)
	if err != nil {
		return fmt.Errorf("削除対象のパスのシンボリックリンク解決に失敗しました: %s: %w", target, err)
	}

	rel, err := filepath.Rel(realBase, realTarget)
	if err != nil {
		return fmt.Errorf("削除対象のパスを基準ディレクトリからの相対パスにできません: %s: %w", target, err)
	}
	if rel == "." {
		return fmt.Errorf("基準ディレクトリ自身は削除できません: %s", target)
	}
	if isOutside(rel) {
		return fmt.Errorf("削除対象のパスが基準ディレクトリ %s の配下にありません: %s", realBase, realTarget)
	}

	// rel の先頭要素が許可サブツリーであることを見る。realTarget は解決済みなので、
	// _work 自体がリンクで外を指していた場合は上の配下判定で既に落ちている。
	top := rel
	if i := strings.IndexRune(rel, filepath.Separator); i >= 0 {
		top = rel[:i]
	}
	for _, allowed := range allowedSubtrees {
		if top == allowed {
			return nil
		}
	}
	return fmt.Errorf("削除が許可されたサブツリー（%s）の外です: %s",
		strings.Join(allowedSubtrees, " / "), realTarget)
}

// hasDotDot はパス要素に ".." があるかを返す。os の正規化に頼らず自前で判定する。
func hasDotDot(path string) bool {
	for _, elem := range strings.Split(filepath.ToSlash(path), "/") {
		if elem == ".." {
			return true
		}
	}
	return false
}

// isOutside は filepath.Rel の結果が基準の外を指しているかを返す。
func isOutside(rel string) bool {
	return rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
