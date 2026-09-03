// Package validcheck は internal/config/edit が huh の Validate へ渡す入口
// （`Validate*`）の回帰テストだけを置く。実行時のコードは含まない。
//
// 親ディレクトリの edit（設定の変更を組み、差分を出し、書く）と分けてあるのは、
// **増え方が違い互いに依存も無い**ためである——edit の側は「何を編集できるか」
// （FR-37 の drop-in、コピー先、ラベルの API 呼び出し）が増えるたびに伸びるが、
// こちらが増えるのは**入力として何を弾くか**が変わったときだけで、根拠は
// internal/config と internal/appconfig の規則である。edit の側の道具
// （`sample` / `write` / `read` は runner を 1 台こしらえる）は入力の検証には
// 要らないので、分けても共有するヘルパは 1 つも無い。
//
// 先例は internal/buildconfig → internal/buildconfig/docscheck である
// （判断は docs/ui/atomic-design.md の「`internal/config/edit` の入力の検証の
// 検査を `validcheck` へ分けた判断」）。
package validcheck
