// Package buildconfigtest は buildconfig の検査群が共有する道具を置く。
//
// `internal/buildconfig`（ビルド設定の検査）・`internal/buildconfig/docscheck`
// （ドキュメントの検査）・`internal/buildconfig/docscheck/linebudget`（行数の予算の検査）は、
// 増え方が違い互いに依存も無いためディレクトリを分けた（Issue #161 と Issue #171。判断は
// atomic-design.md の「`internal/buildconfig` を 2 つに分けた判断」と「`docscheck` から
// 行数の予算の検査を分けた判断」）。分けると _test.go の中のヘルパは共有できなくなるので、
// **3 者のどれからも import できる**通常のパッケージをここに置く。**同じ形の先例は `page/pagetest` /
// `organism/table/tabletest` / `setup/setuptest` である**（本番から import できて
// しまう代償も同じなので、`page/pagetest/import_test.go` の `fixtures` へ登録して
// 検査の網に入れてある）。
//
// 置くのは**複数が使う道具だけ**にすること。1 つしか使わない道具をここへ寄せると、
// 分けた意味（増え方の違うものを別々に育てる）が消え、このディレクトリがもう 1 つの
// 逼迫する置き場になる。
package buildconfigtest

import (
	"os"
	"path/filepath"
	"strconv"
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

// Atoi は表から読んだ数値文字列を int にする。変換できなければテストを落とす。
//
// 表（改訂履歴の版番号・行数表の行数と残り）を正規表現で拾う検査が共有する。
// 拾えた時点で数字だけであることは正規表現が保証しているので、ここで失敗するのは
// 正規表現と呼び出し側が食い違ったときであり、黙って 0 として扱うと検査が緑のまま
// 意味を失う。
func Atoi(t *testing.T, s string) int {
	t.Helper()

	n, err := strconv.Atoi(s)
	if err != nil {
		t.Fatalf("数値にできない文字列 %q: %v", s, err)
	}
	return n
}
