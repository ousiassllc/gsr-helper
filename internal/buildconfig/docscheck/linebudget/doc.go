// Package linebudget は「行数の予算」の不変条件を守る検査だけを置く。実行時のコードは含まない。
//
// 対象は docs/ui/atomic-design.md の「行数の予算」の 2 つの行数表と、そこが定める散文の規約
// （現在の行数を数で書いてよいのは行数表だけである）を守る 2 文書——docs/ui/atomic-design.md と
// docs/components/overview.md——の散文である。**散文の側が 2 文書なのは Issue #169 で対象を
// 広げたからである。** 表が `go tool linterly check` の実測とずれても、散文が古い実測値を
// 現在形で語り続けても、通常のテストでは検知できない。
//
// 親の docscheck から分けてあるのは、**増え方が違い互いに依存も無い**ためである
// （Issue #171。判断は docs/ui/atomic-design.md の「`docscheck` から行数の予算の検査を分けた判断」）。
// docscheck に残るのは依存グラフ・nolint 棚卸し・改訂履歴・設定ファイルの埋め込み・不変条件
// 一覧表の検査で、こちらは行数の予算を触るたびに増える——実際 Issue #164 / #167 / #169 が
// 続けて検査を足しており、この 4 ファイルが docscheck のなかで最も速く伸びていた。
// 共有していたのは表から読んだ数値を int にするヘルパ 1 つだけで、それは buildconfigtest へ出した。
package linebudget
