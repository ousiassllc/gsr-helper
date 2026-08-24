package docscheck

import (
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/buildconfig/buildconfigtest"
)

// nolintDocRow は docs/environment/setup.md の「抑制の方針」が載せている棚卸しの表の行。
// 例: | `internal/audit/open.go` | 2 | 変数を使ったファイル読み取り（G304） | … |
var nolintDocRow = regexp.MustCompile("^\\s*\\|\\s*`([^`]+\\.go)`\\s*\\|\\s*(\\d+)\\s*\\|")

// 仕様書の nolint 棚卸しは、現在のツリーの実態と一致していなければならない。
//
// この表はファイルの移動・分割に追随しそこねる。実際、`internal/runner` の分割で
// `procs.go` が `procs/procs.go` へ移り、`systemd.go` の暫定抑制が除去された後も、
// 仕様書は存在しないファイルの存在しない抑制を挙げたままだった（Issue #44）。
// コンパイルエラーにも通常のテストにもならないため、ここで機械的に突き合わせる。
func TestSetupDocNolintInventoryMatchesTree(t *testing.T) {
	root := buildconfigtest.RepoRoot(t)

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

// countNolintInTree は本番コード（_test.go 以外）の抑制ディレクティブをファイルごとに数える。
//
// 数えるのは**実際のディレクティブだけ**である。判定は golangci-lint 自身の規則
// （`pkg/result/processors/nolint.go`）に合わせ、コメント本文を
// `strings.TrimLeft(text, "/ ")` した結果が `nolint` で始まるものだけを 1 件と数える。
// したがって、コメントの途中で //nolint に言及しているだけの散文（この docscheck
// パッケージの doc コメントがまさにそれである）も、文字列リテラルの中の //nolint も
// 数えない。逆に golangci-lint が抑制として尊重する `// nolint:gosec`（先頭に空白の
// ある形）は数えるので、ファイルのバイト列を正規表現で走査していた以前より
// 取りこぼしは少ない（Issue #165）。
//
// _test.go を除くのは、`.golangci.yml` が `_test.go` に対する errcheck / gosec を
// 除外しており抑制を書く必要が無いためである（上の判定とは独立した第 2 の理由で、
// 実際 `internal/buildconfig/golangci_test.go` のフィクスチャ文字列に現れる //nolint は
// 文字列リテラルなので、どちらの理由でも数に入らない）。
func countNolintInTree(t *testing.T, root string) map[string]int {
	t.Helper()

	fset := token.NewFileSet()
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
		file, err := parser.ParseFile(fset, path, nil, parser.ParseComments|parser.SkipObjectResolution)
		if err != nil {
			// ツリーに構文エラーのある .go があればビルド自体が壊れている。
			t.Fatalf("Go ソースとして解析できない（%s）: %v", path, err)
		}
		n := 0
		for _, group := range file.Comments {
			for _, comment := range group.List {
				if strings.HasPrefix(strings.TrimLeft(comment.Text, "/ "), "nolint") {
					n++
				}
			}
		}
		if n > 0 {
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

// nolintFixtureSource は countNolintInTree に数えさせる 4 通りを 1 ファイルに詰めた
// フィクスチャ。散文の言及と文字列リテラルは数えず、本物のディレクティブ 2 つだけを
// 数えることを期待する。
const nolintFixtureSource = `package fixture

// Doc は //nolint:gosec のような抑制の書き方を説明するだけの散文であり、
// 抑制そのものではない。
func Doc() string {
	//nolint:gosec // 本物の抑制（理由つき）
	mention := "//nolint:gosec"

	// nolint:errcheck // 先頭に空白のある本物の抑制
	return mention
}
`

// countNolintInTree が数えるのは実際の抑制ディレクティブだけで、散文の言及や文字列
// リテラルは数えない、という約束の回帰テスト。
//
// ファイルのバイト列を正規表現で走査していたころは、本番コードの doc コメントに抑制
// ディレクティブの文字列を書いた瞬間に TestSetupDocNolintInventoryMatchesTree が
// 偽陽性で落ちた。実際 `internal/buildconfig/docscheck/doc.go` は回避のため先頭の
// `//` を落として書かれており、その回避策はどこにも記録されていなかった（Issue #165）。
func TestCountNolintInTreeIgnoresDocCommentsAndStringLiterals(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "fixture.go"), []byte(nolintFixtureSource), 0o600); err != nil {
		t.Fatalf("フィクスチャを書けない: %v", err)
	}

	got := countNolintInTree(t, root)
	want := map[string]int{"fixture.go": 2}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("countNolintInTree = %v, want %v", got, want)
	}
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
