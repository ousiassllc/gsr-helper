// Package buildconfigtest は buildconfig の検査群が共有する道具を置く。
//
// `internal/buildconfig`（ビルド設定の検査）と `internal/buildconfig/docscheck`
// （ドキュメントの検査）は、増え方が違い互いに依存も無いためディレクトリを分けた
// （Issue #161。判断は atomic-design.md の「`internal/buildconfig` を 2 つに分けた判断」）。
// 分けると _test.go の中のヘルパは共有できなくなるので、両方から import できる
// 通常のパッケージをここに置く。**同じ形の先例は `page/pagetest` /
// `organism/table/tabletest` / `setup/setuptest` である**（本番から import できて
// しまう代償も同じなので、`page/pagetest/import_test.go` の `fixtures` へ登録して
// 検査の網に入れてある）。
//
// 置くのは**両方が使う道具だけ**にすること。片方しか使わない道具をここへ寄せると、
// 分けた意味（増え方の違う 2 つを別々に育てる）が消え、このディレクトリが 3 つ目の
// 逼迫する置き場になる。
package buildconfigtest

import (
	"os"
	"path/filepath"
	"testing"
)

// RepoRoot はテスト実行ディレクトリから遡り、go.mod を持つリポジトリルートを返す。
//
// 検査の対象（Makefile・CI ワークフロー・lint 設定・docs/ 配下）はいずれもリポジトリ
// ルートからの相対パスにあり、テストの実行ディレクトリはパッケージごとに違う。
func RepoRoot(t *testing.T) string {
	t.Helper()

	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("カレントディレクトリを取得できない: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod を持つリポジトリルートが見つからない")
		}
		dir = parent
	}
}
