package buildconfig

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// nolintDocRow は docs/environment/setup.md の「抑制の方針」が載せている棚卸しの表の行。
// 例: | `internal/audit/open.go` | 2 | 変数を使ったファイル読み取り（G304） | … |
var nolintDocRow = regexp.MustCompile("^\\s*\\|\\s*`([^`]+\\.go)`\\s*\\|\\s*(\\d+)\\s*\\|")

// nolintDirective は行コメントとして書かれた抑制。//nolint:gosec のようにリンター名が
// 続く形と、リンター名のない //nolint 単独の形の両方を数える。
// nolintlint（require-specific）が後者を落とすため、実際に残るのは前者だけである。
var nolintDirective = regexp.MustCompile(`//nolint\b`)

// 仕様書の nolint 棚卸しは、現在のツリーの実態と一致していなければならない。
//
// この表はファイルの移動・分割に追随しそこねる。実際、`internal/runner` の分割で
// `procs.go` が `procs/procs.go` へ移り、`systemd.go` の暫定抑制が除去された後も、
// 仕様書は存在しないファイルの存在しない抑制を挙げたままだった（Issue #44）。
// コンパイルエラーにも通常のテストにもならないため、ここで機械的に突き合わせる。
func TestSetupDocNolintInventoryMatchesTree(t *testing.T) {
	root := repoRoot(t)

	tree := countNolintInTree(t, root)
	doc := parseNolintInventory(t, root)

	for _, path := range sortedKeys(tree) {
		switch got, ok := doc[path]; {
		case !ok:
			t.Errorf("%s に %d 件の //nolint があるが、setup.md の棚卸しの表に無い", path, tree[path])
		case got != tree[path]:
			t.Errorf("%s の //nolint は %d 件だが、setup.md の棚卸しは %d 件としている", path, tree[path], got)
		}
	}
	for _, path := range sortedKeys(doc) {
		if _, ok := tree[path]; !ok {
			t.Errorf("setup.md の棚卸しが %s を挙げているが、そのファイルに //nolint は無い（移動・除去に追随できていない）", path)
		}
	}
}

// countNolintInTree は本番コード（_test.go 以外）の //nolint をファイルごとに数える。
//
// _test.go を除くのは、`.golangci.yml` が `_test.go` に対する errcheck / gosec を
// 除外しており抑制を書く必要が無いためである。実際 `internal/buildconfig` の
// フィクスチャ文字列に現れる //nolint はこのリポジトリのコードに対する抑制ではなく、
// 数えると棚卸しの意味が壊れる。
func countNolintInTree(t *testing.T, root string) map[string]int {
	t.Helper()

	counts := make(map[string]int)
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			// 隠しディレクトリ（.git / 入れ子の worktree を置く .claude）と
			// testdata は本番コードではないので降りない。
			if name := d.Name(); path != root && (strings.HasPrefix(name, ".") || name == "testdata") {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if n := len(nolintDirective.FindAll(body, -1)); n > 0 {
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			counts[filepath.ToSlash(rel)] = n
		}
		return nil
	})
	if err != nil {
		t.Fatalf("ツリーを走査できない: %v", err)
	}
	return counts
}

// parseNolintInventory は setup.md の棚卸しの表を「パス → 件数」に読み取る。
func parseNolintInventory(t *testing.T, root string) map[string]int {
	t.Helper()

	body, err := os.ReadFile(filepath.Join(root, "docs", "environment", "setup.md"))
	if err != nil {
		t.Fatalf("setup.md を読めない: %v", err)
	}

	inventory := make(map[string]int)
	for _, line := range strings.Split(string(body), "\n") {
		m := nolintDocRow.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		n, err := strconv.Atoi(m[2])
		if err != nil {
			t.Fatalf("棚卸しの件数を読めない（%q）: %v", line, err)
		}
		inventory[m[1]] = n
	}
	if len(inventory) == 0 {
		t.Fatal("setup.md から nolint 棚卸しの表を読み取れなかった（表の形が変わった可能性がある）")
	}
	return inventory
}

func sortedKeys(m map[string]int) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
