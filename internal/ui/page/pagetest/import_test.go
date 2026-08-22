package pagetest_test

import (
	"errors"
	"go/build"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// モジュールと、依存の向きを検査する対象のパス。
const (
	modulePath = "github.com/ousiassllc/gsr-helper"
	uiPage     = modulePath + "/internal/ui/page"
	selfPath   = uiPage + "/pagetest"
	tabsetPath = modulePath + "/internal/ui/tabset"
)

// shared は page/ 直下にあってタブではないパッケージ。
//
// `page/` は「1 ディレクトリ 1 タブ」ではない。タブが共用する部品（操作の判定・
// 詳細画面）とテスト用フィクスチャも同じ階層に並ぶ（atomic-design.md のディレクトリ
// 構成）。ここに載っていない page/<名前> はタブとして扱う。**新しいタブを足しても
// 自動で検査の対象になり、共有部品を足すときだけここへの追記という明示的な判断が
// 要る**、という向きにしてある。
var shared = map[string]bool{
	"action":       true,
	"runnerdetail": true,
	// runnerop は Runners / Jobs が共用するサービス制御の制御部であり、タブではない。
	"runnerop": true,
	"pagetest": true,
}

// テスト用の道具は本番の経路から import されない。
//
// このパッケージは _test.go ではなく通常のパッケージである（タブ 1 枚ごとに
// パッケージが分かれるため、_test.go に置いた道具を他のパッケージから import
// できない。pagetest.go の doc）。その代償として、本番コードからも普通に import
// できてしまう。中身は exec.NewFake() と固定フィクスチャなので、混入すればテスト用の
// 偽物がそのまま製品に載る。
//
// **規約ではなく検査で止める。** ビルドタグでは「テストからは使えてタブからは使えない」
// を表せず、置いてあるだけの規約は次にタブを足す Issue で破られる（Issue #45）。
func TestNoProductionCodeImportsPagetest(t *testing.T) {
	for pkg, imports := range productionImports(t) {
		for _, imp := range imports {
			if imp == selfPath {
				t.Errorf("本番コード %s が %s を import している（テスト用の道具は _test.go からのみ使うこと）", pkg, selfPath)
			}
		}
	}
}

// タブ 1 枚を import してよいのは tabset だけである。
//
// atomic-design.md は「`page/<tab>` → `page` の一方向依存が Go の import で強制され、
// タブ同士が参照し合えなくなる」と書いていたが、**これは成り立たない**。Go が禁じるのは
// 循環（`page` → `page/<tab>`）だけで、新しいタブが `page/runners` を直に import しても
// 通る。タブ間で共有する状態は親 Model のみが持つ、という本書の中心的な規則が規約に
// しか無い状態だった（Issue #45）。
//
// 「親 Model は個別のタブを知らない」も同じ検査で守れる。タブを知るのは tabset だけ
// であり、`ui` 直下が `page/runners` を import し始めたらここで落ちる。
func TestOnlyTabsetImportsTabs(t *testing.T) {
	all := productionImports(t)

	tabs := make(map[string]bool)
	for pkg := range all {
		if name, ok := tabName(pkg); ok && !shared[name] {
			tabs[pkg] = true
		}
	}
	if len(tabs) == 0 {
		t.Fatal("タブのパッケージを 1 つも見つけられなかった（検査が空振りしている）")
	}

	for pkg, imports := range all {
		if pkg == tabsetPath {
			continue
		}
		for _, imp := range imports {
			if tabs[imp] && imp != pkg {
				t.Errorf("%s がタブ %s を直に import している（タブを知ってよいのは %s だけ）", pkg, imp, tabsetPath)
			}
		}
	}
}

// tabName は import パスが page/ 直下のものならその名前を返す。
func tabName(pkg string) (string, bool) {
	rest, ok := strings.CutPrefix(pkg, uiPage+"/")
	if !ok || strings.Contains(rest, "/") {
		return "", false
	}
	return rest, true
}

// productionImports はモジュール内の各パッケージの、本番ファイルだけの import を返す。
//
// build.Package.Imports は GoFiles の import だけを持つ。テストの import
// （TestImports / XTestImports）は別のフィールドなので、ここには現れない。
func productionImports(t *testing.T) map[string][]string {
	t.Helper()

	root := moduleRoot(t)
	out := make(map[string][]string)

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			return nil
		}
		// .git や隠しディレクトリ、フィクスチャ置き場は見ない。
		if name := d.Name(); path != root && (strings.HasPrefix(name, ".") || name == "testdata") {
			return fs.SkipDir
		}

		pkg, perr := build.ImportDir(path, 0)
		if perr != nil {
			// Go ファイルが無いディレクトリ（docs/ など）は対象外。
			var noGo *build.NoGoError
			if errors.As(perr, &noGo) {
				return nil
			}
			return perr
		}

		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		importPath := modulePath
		if rel != "." {
			importPath += "/" + filepath.ToSlash(rel)
		}
		out[importPath] = pkg.Imports
		return nil
	})
	if err != nil {
		t.Fatalf("ツリーを走査できない: %v", err)
	}
	// 走査そのものが空振りしていないことを確かめる（ディレクトリ構成が変わって
	// 1 つも見ていないのに緑、という状態を作らない）。
	if len(out) == 0 {
		t.Fatal("Go パッケージを 1 つも検査していない")
	}
	return out
}

// moduleRoot は go.mod のあるディレクトリを返す。
func moduleRoot(t *testing.T) string {
	t.Helper()

	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("作業ディレクトリを取得できない: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod が見つからない")
		}
		dir = parent
	}
}
