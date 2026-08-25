package docscheck

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"github.com/ousiassllc/gsr-helper/internal/buildconfig/buildconfigtest"
)

// invariantTableHeader は setup.md の「設定ファイルの不変条件をテストで守る」節が持つ
// 一覧表の見出し行。この行から最初の非 `|` 行までを表とみなす。
const invariantTableHeader = "| 守っている不変条件 | 破ったときに落ちるテスト |"

// invariantTableDirs は一覧表が範囲とする 2 ディレクトリ（リポジトリルートからの相対）。
//
// setup.md の「**この表が挙げるのは `internal/buildconfig` とその `docscheck` に置いた
// ものだけである。**」で始まる段落がこの 2 つを定めている。サブディレクトリは辿らない
// ——`internal/buildconfig/buildconfigtest` は両者が共有する道具（`RepoRoot`）の置き場で
// あって検査ではないので、表にも載らないし、ここでも数えない。
var invariantTableDirs = []string{
	filepath.Join("internal", "buildconfig"),
	filepath.Join("internal", "buildconfig", "docscheck"),
}

// invariantTableTestName は表のセルに現れるバッククォートで囲んだテスト名。
// 区切り行（`|---|---|`）にはバッククォートが無いので自然に読み飛ばされる。
var invariantTableTestName = regexp.MustCompile("`(Test[A-Za-z0-9_]*)`")

// setup.md の不変条件テスト一覧表は、実装にあるテストの集合と一致していなければならない。
//
// この表は「設定やドキュメントに新しい取り決めを入れたときは、同じ場所にテストを足す」と
// 自ら定めて一覧への記載を求めていながら、実在する検査のうち 27 本を取りこぼしていた
// （Issue #166）。**手で写した一覧は黙って古くなる**——Issue #164 / #167 / #169 が行数の
// 実測値の写しに対して繰り返し確かめたのと同じ形が、この表自身にも起きていた。表に無い
// 検査は、次に対象を触る Issue から見えない。
//
// **突き合わせは両方向で行う。** 片方向（表に無い実装を探すだけ）だと、テストを消したり
// 改名したりしたときに表が存在しないテストを挙げ続ける。これは机上の話ではなく、
// TestSetupDocNolintInventoryMatchesTree が `internal/runner` の分割で実際に踏んだ形
// （仕様書が存在しないファイルの存在しない抑制を挙げたまま残った。Issue #44）である。
//
// 対象を `internal/buildconfig` 直下と `internal/buildconfig/docscheck` の 2 つに限る根拠は
// setup.md の「**この表が挙げるのは `internal/buildconfig` とその `docscheck` に置いた
// ものだけである。**」で始まる段落にある。この段落が表の範囲を定めているからこそ、
// 集合の一致という形で機械的に検査できる。
func TestSetupDocInvariantTableListsEveryTest(t *testing.T) {
	root := buildconfigtest.RepoRoot(t)

	impl := invariantTestFuncs(t, root)
	doc := invariantTableEntries(t, root)

	for _, name := range sortedTestNames(impl) {
		if !doc[name] {
			t.Errorf("%s が setup.md の不変条件テスト一覧表に無い（何を守る検査かを 1 行で書いて表へ行を足すこと）", name)
		}
	}
	for _, name := range sortedTestNames(doc) {
		if !impl[name] {
			t.Errorf("setup.md の不変条件テスト一覧表が %s を挙げているが、そのテストは実装に無い（除去・改名に追随できていない。表から行を落とすか、新しい名前へ直すこと）", name)
		}
	}
}

// invariantTestFuncs は invariantTableDirs のテスト関数の名前を集める。
//
// 集めるのは**最上位の関数宣言のうち、名前が Test で始まり、レシーバを持たず、引数が
// `*testing.T` 1 つのもの**である。`grep '^func Test'` ではなく go/parser で走査するのは、
// テストがフィクスチャとして書き出す Go ソースの文字列リテラル（`internal/buildconfig` の
// makefile 系の検査が一時モジュールへ書き出す `TestRace` など）を実在のテストと取り違え
// ないためである。文字列リテラルの中身は宣言ではないので、構文木を見れば混ざらない。
func invariantTestFuncs(t *testing.T, root string) map[string]bool {
	t.Helper()

	fset := token.NewFileSet()
	names := make(map[string]bool)
	for _, dir := range invariantTableDirs {
		entries, err := os.ReadDir(filepath.Join(root, dir))
		if err != nil {
			t.Fatalf("%s を読めない: %v", dir, err)
		}
		for _, entry := range entries {
			// サブディレクトリへは降りない（表の範囲は 2 ディレクトリの直下だけ）。
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), "_test.go") {
				continue
			}
			path := filepath.Join(root, dir, entry.Name())
			file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
			if err != nil {
				t.Fatalf("Go ソースとして解析できない（%s）: %v", path, err)
			}
			for _, decl := range file.Decls {
				if fn, ok := decl.(*ast.FuncDecl); ok && isTestFuncDecl(fn) {
					names[fn.Name.Name] = true
				}
			}
		}
	}
	if len(names) == 0 {
		t.Fatalf("%v からテスト関数を 1 件も見つけられなかった（検査の置き場が変わった可能性がある）", invariantTableDirs)
	}
	return names
}

// isTestFuncDecl は宣言が `func TestXxx(t *testing.T)` の形かどうかを判定する。
func isTestFuncDecl(fn *ast.FuncDecl) bool {
	if fn.Recv != nil || !isGoTestName(fn.Name.Name) {
		return false
	}
	params := fn.Type.Params.List
	if len(params) != 1 {
		return false
	}
	// ベンチマーク（*testing.B）やファズ（*testing.F）は `go test -list '^Test'` の
	// 対象ではないので、引数の型まで見て落とす。
	star, ok := params[0].Type.(*ast.StarExpr)
	if !ok {
		return false
	}
	sel, ok := star.X.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	return ok && pkg.Name == "testing" && sel.Sel.Name == "T"
}

// invariantTableEntries は setup.md の一覧表が挙げるテスト名を集める。
func invariantTableEntries(t *testing.T, root string) map[string]bool {
	t.Helper()

	body, err := os.ReadFile(filepath.Join(root, "docs", "environment", "setup.md"))
	if err != nil {
		t.Fatalf("setup.md を読めない: %v", err)
	}

	lines := strings.Split(string(body), "\n")
	start := -1
	for i, line := range lines {
		if strings.TrimSpace(line) == invariantTableHeader {
			start = i
			break
		}
	}
	if start < 0 {
		t.Fatalf("setup.md に一覧表の見出し行 %q が無い（表の形が変わった可能性がある）", invariantTableHeader)
	}

	names := make(map[string]bool)
	for _, line := range lines[start+1:] {
		if !strings.HasPrefix(strings.TrimSpace(line), "|") {
			break
		}
		for _, m := range invariantTableTestName.FindAllStringSubmatch(line, -1) {
			names[m[1]] = true
		}
	}
	if len(names) == 0 {
		t.Fatal("setup.md の一覧表から 1 件もテスト名を読み取れなかった（表の形が変わった可能性がある）")
	}
	return names
}

func sortedTestNames(m map[string]bool) []string {
	names := make([]string, 0, len(m))
	for name := range m {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// isGoTestName は名前が `go test` にとってのテスト関数名かどうかを返す。
//
// 判定は `testing` の `isTest` と同じ——`Test` の次の文字が小文字なら**テストでは
// ない**（`Testify` のような名前がそれである）。`Test` で始まるかどうかだけで見ると、
// この検査だけが `go test -list` の集合より広くなり、**一覧表に「足しても `go test`
// が走らせないもの」を足させる**ことになる。表が `go test` の集合と一致することを
// 見るのが本検査の目的なので、判定の規則も toolchain 側に合わせる。
func isGoTestName(name string) bool {
	const prefix = "Test"
	if !strings.HasPrefix(name, prefix) {
		return false
	}
	if len(name) == len(prefix) {
		return true
	}
	r, _ := utf8.DecodeRuneInString(name[len(prefix):])
	return !unicode.IsLower(r)
}
