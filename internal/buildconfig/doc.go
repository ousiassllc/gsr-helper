// Package buildconfig はビルド設定ファイル（Makefile・CI ワークフロー・lint 設定・Git Hooks 設定・
// 依存更新設定 .github/dependabot.yml）の不変条件を守る回帰テストだけを置く。実行時のコードは含まない。
//
// 設定ファイルはリポジトリルートにありコンパイル対象にもならないため、壊れても
// 通常のテストでは検知できない。ここでは一時ディレクトリに最小のモジュールを作り、
// リポジトリルートの Makefile を実際に実行して挙動を固定する。
//
// ドキュメント（docs/ 配下の Markdown）の不変条件は、同じ性格の検査でありながら
// 増え方が違い互いに依存も無いため、サブディレクトリの docscheck へ分けてある
// （Issue #161。判断は docs/ui/atomic-design.md の「`internal/buildconfig` を 2 つに
// 分けた判断」）。そのうち行数の予算の検査だけは、伸びが速く自己完結していたので
// さらに docscheck/linebudget へ分けてある（Issue #171。判断は同じ文書の
// 「`docscheck` から行数の予算の検査を分けた判断」）。この 3 つのうち複数が使う道具
// だけを buildconfigtest に置く。
package buildconfig
