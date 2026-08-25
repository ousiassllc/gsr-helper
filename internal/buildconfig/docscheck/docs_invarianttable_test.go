package docscheck

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"github.com/ousiassllc/gsr-helper/internal/buildconfig/buildconfigtest"
)

// invariantTableHeader は setup.md の「設定ファイルの不変条件をテストで守る」節が持つ
// 一覧表の見出し行。この行から最初の非 `|` 行までを表とみなす。
const invariantTableHeader = "| 守っている不変条件 | 破ったときに落ちるテスト |"

// invariantTableRootDir は検査を置くディレクトリの親（リポジトリルートからの相対）。
const invariantTableRootDir = "internal/buildconfig"

// invariantTableDirs は一覧表が範囲とする 2 ディレクトリ（リポジトリルートからの相対）。
//
// setup.md の「**この表が挙げるのは `internal/buildconfig` とその `docscheck` に置いた
// ものだけである。**」で始まる段落がこの 2 つを定めている。`buildconfigtest` は両者が
// 共有する道具（`RepoRoot`）の置き場であって検査ではないので、表にも載らないし、ここにも
// 挙げない。**この一覧が実態から遅れたことは invariantTestFiles が知らせる**——`docscheck`
// 自体が Issue #161 の分割で生まれており、再分割は現実に起こりうる。
var invariantTableDirs = []string{
	filepath.FromSlash(invariantTableRootDir),
	filepath.Join(filepath.FromSlash(invariantTableRootDir), "docscheck"),
}

// invariantTableTestName は表のセルに現れるバッククォートで囲んだテスト名。
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
// ものだけである。**」で始まる段落にある。この段落が表の範囲を定めているからこそ、集合の
// 一致という形で機械的に検査できる。**その範囲自体のずれは invariantTestFiles が見る。**
func TestSetupDocInvariantTableListsEveryTest(t *testing.T) {
	root := buildconfigtest.RepoRoot(t)

	impl := invariantTestFuncs(t, root)
	doc := invariantTableEntries(t, root)

	for _, name := range slices.Sorted(maps.Keys(impl)) {
		if !doc[name] {
			t.Errorf("%s が setup.md の不変条件テスト一覧表に無い（何を守る検査かを 1 行で書いて表へ行を足すこと）", name)
		}
	}
	for _, name := range slices.Sorted(maps.Keys(doc)) {
		if !impl[name] {
			t.Errorf("setup.md の不変条件テスト一覧表が %s を挙げているが、そのテストは実装に無い（除去・改名に追随できていない。表から行を落とすか、新しい名前へ直すこと）", name)
		}
	}
}

// invariantTestFiles は invariantTableRootDir 配下の `_test.go` を**深さを問わず**集め、
// 置き場が invariantTableDirs に無ければ落とす。
//
// **一覧が直書きだけだと、範囲の外に検査が生まれても表も検査も黙る。** 3 つ目の検査
// ディレクトリ（`internal/buildconfig/newcheck`）を作ってテストを置いても緑のままで、
// `buildconfigtest` へテストを置いた場合も同じだった——setup.md が「buildconfigtest は
// 検査を持たない」と現在形で述べている前提そのものが無検査だったということである。
//
// **深さを問わないのは、この検査の動機がまさに子ディレクトリへの分割だからである。**
// 直下を 1 段だけ読んでいた頃は `docscheck/sub` も `newcheck/sub` も素通りしており、
// `docscheck` 自身が Issue #161 で親から分かれて生まれた先例のとおり、次の分割も
// 子ディレクトリの形で来る。動機の形そのものが死角に落ちていた。
func invariantTestFiles(t *testing.T, root string) []string {
	t.Helper()

	base := filepath.FromSlash(invariantTableRootDir)
	var paths []string
	walkErr := filepath.WalkDir(filepath.Join(root, base),
		func(path string, entry fs.DirEntry, err error) error {
			// 走査エラーを左端に置く（そのとき entry は nil でありうる）。
			if err != nil || entry.IsDir() || !strings.HasSuffix(entry.Name(), "_test.go") {
				return err
			}
			dir, err := filepath.Rel(root, filepath.Dir(path))
			if err != nil {
				return err
			}
			if !slices.Contains(invariantTableDirs, dir) {
				t.Fatalf("%s がテストを持っているが invariantTableDirs に無い"+
					"——一覧表の範囲（setup.md の「**この表が挙げるのは `internal/buildconfig` と"+
					"その `docscheck` に置いたものだけである。**」で始まる段落）と検査の範囲がずれた。"+
					"このディレクトリを invariantTableDirs へ足して表にも行を足すか、"+
					"setup.md の範囲の記述を直すこと", dir)
			}
			paths = append(paths, path)
			return nil
		})
	if walkErr != nil {
		t.Fatalf("%s を辿れない: %v", base, walkErr)
	}
	return paths
}

// invariantTestFuncs は invariantTestFiles が集めたファイルのテスト関数名を集める。
//
// **集める側も深さを問わない**——範囲の検査だけを深くしても、集める集合が 1 段のままでは
// 一致の検査が噛み合わない。どちらも invariantTestFiles の 1 回の走査から取る。
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
	for _, path := range invariantTestFiles(t, root) {
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
	for i, line := range lines[start+1:] {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "|") {
			break
		}
		// **テスト名は右カラム（「破ったときに落ちるテスト」）だけから読む。** 行全体に
		// 掛けていた頃は、左カラム（説明）でテスト名に言及しただけで「表に載っている」と
		// 数えた——ある行を丸ごと消してその名前を別の行の説明カラムへ書き足すだけで、
		// 検査が緑のまま通ることを実測している。区切り行（`|---|---|`）は 2 セルに割れ、
		// バッククォートを持たないので自然に読み飛ばされる。
		cells := strings.Split(strings.TrimSuffix(strings.TrimPrefix(trimmed, "|"), "|"), "|")
		if len(cells) != 2 {
			t.Fatalf("setup.md:%d: 一覧表の行のセルが %d 個ある（2 個のはず）: %s",
				start+2+i, len(cells), trimmed)
		}
		for _, m := range invariantTableTestName.FindAllStringSubmatch(cells[1], -1) {
			names[m[1]] = true
		}
	}
	if len(names) == 0 {
		t.Fatal("setup.md の一覧表から 1 件もテスト名を読み取れなかった（表の形が変わった可能性がある）")
	}
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
