package molecule

import (
	"github.com/ousiassllc/gsr-helper/internal/ui/token"
)

// LogLine は Logs タブの本文 1 行を返す（[FR-25] の `ERROR` / `WARN` の強調表示）。
//
// 重大度そのものは受け取らない。ドメインの型を molecule 以下へ持ち込まないためであり
// （atomic-design.md の依存の規則）、重大度から表示上の役割への対応づけは page が持つ。
//
// **一致部分のハイライトは行わない。** フィルタは一致しない行を落とすので、本文に
// 並ぶ行はすべて一致している。行の中のどこが一致したかを塗り分けても、`ERROR` /
// `WARN` の強調と色が重なって読みにくくなるだけである（FR-25 が求めるのは
// 「正規表現によるフィルタ」と「`ERROR` / `WARN` の強調表示」の 2 つで、
// 一致部分の強調は含まない）。
//
// 幅に合わせた切り詰めもしない。本文は `bubbles/viewport` が折り返して描くため、
// ここで切ると画面外の桁が読めなくなる。
func LogLine(text string, role token.RoleToken, s token.Styles) string {
	if role == token.RolePlain {
		// 素通しのスタイルでも Render は文字列を作り直す。強調しない行が本文の
		// 大半を占めるので、その 1 回を省く。
		return text
	}
	return s.Style(role).Render(text)
}
