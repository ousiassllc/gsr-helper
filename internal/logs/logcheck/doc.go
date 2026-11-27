// Package logcheck は internal/logs の公開 API（List / ParseWorker / Tail）を外から
// 検査するテストだけを置く。実行時のコードは含まない。
//
// 親ディレクトリの logs（ログの列挙・追従・解析）と分けてあるのは、1 ディレクトリ
// 2000 行の上限を空けるためである。ここに置くのは unexported な名前に触れない検査
// だけで、logs 側へ戻す理由になる共有ヘルパは無い。
//
// 先例は internal/config/edit → internal/config/edit/validcheck である（判断は
// docs/ui/atomic-design.md の「行数の予算」）。
package logcheck
