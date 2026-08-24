// Package docscheck はドキュメント（docs/ 配下の Markdown）の不変条件を守る回帰
// テストだけを置く。実行時のコードは含まない。
//
// ドキュメントはコンパイル対象ではないため、仕様書のコードブロックが設定ファイルの実体から乖離しても、
// 改訂履歴の版番号が重複・逆順になっても、docs/environment/setup.md の nolint 抑制の棚卸し表が
// ツリーの実態とずれても、依存グラフの図が実装の import とずれても、通常のテストでは検知できない。
//
// 親ディレクトリの buildconfig（ビルド設定の検査）と分けてあるのは、**増え方が違い
// 互いに依存も無い**ためである——ビルド設定の検査は Makefile / CI / lint 設定を触る
// たびに増え、こちらは文書の取り決めを増やすたびに増える。共有していたのは
// リポジトリルートを求めるヘルパ 1 つだけで、それは buildconfigtest へ出した
// （Issue #161。判断は docs/ui/atomic-design.md の「`internal/buildconfig` を 2 つに
// 分けた判断」）。
package docscheck
