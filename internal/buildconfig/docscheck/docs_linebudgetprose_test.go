package docscheck

import (
	"regexp"
	"strings"
	"testing"
)

// proseBudgetPair は散文の「N 行・残り M 行」「N 行（残り M 行」という主張を拾う。
var proseBudgetPair = regexp.MustCompile(`(\d+)\s*行(?:・|（)残り\s*(-?\d+)\s*行`)

// proseBudgetRemainder は行数を伴わない「残り M 行」「残りは M 行」を拾う。
//
// 末尾の `行` を必須にしてあるのは、`296 行（残り 4）` のようにファイル側の予算
// （1 ファイル 300 行）を指す書き方をわざと外すためである。ディレクトリの表と
// 突き合わせるべき数だけを拾う。
var proseBudgetRemainder = regexp.MustCompile(`残り(?:は)?\s*(-?\d+)\s*行`)

// 散文が現在形で主張する行数・残りも、行数表と一致していなければならない。
//
// 実測値は表・散文・改訂履歴の 3 か所に写されているのに、これまで検査されていたのは
// どこでもなかった。表だけを実測に合わせても、散文は古い数を現在形で語り続ける——
// 実際、散文が後続の Issue に示した予算は本当の値より 3 行多かった（Issue #164）。
//
// **過去の値を書くときは同じ文に `当時` か `時点では` を入れること。** それがこの
// 検査の唯一の除外であり、規約は本書の「行数表と散文は実測に合わせる」に書いてある。
func TestLineBudgetProseMatchesTables(t *testing.T) {
	uiRows, nonUIRows := parseBudgetTables(t)
	rows := append(append(make([]budgetRow, 0, len(uiRows)+len(nonUIRows)), uiRows...), nonUIRows...)

	var claims int
	for _, sentence := range budgetProseSentences(t) {
		// 過去の値は当時の Issue に紐づく記録なので、現在の実測へ寄せてはならない。
		if strings.Contains(sentence, "当時") || strings.Contains(sentence, "時点では") {
			continue
		}
		claims += checkProseBudgetClaims(t, sentence, rows)
	}
	// 表の切り落としが文書ごと食い潰しても緑になるのを防ぐ。
	if claims == 0 {
		t.Fatal("散文から行数の主張を 1 件も拾えない（表の切り落としが広すぎる可能性がある）")
	}
}

// checkProseBudgetClaims は 1 文の中の行数の主張を表と突き合わせ、拾った件数を返す。
func checkProseBudgetClaims(t *testing.T, sentence string, rows []budgetRow) int {
	t.Helper()

	var claims int
	for _, m := range proseBudgetPair.FindAllStringSubmatch(sentence, -1) {
		claims++
		lines, remaining := atoi(t, m[1]), atoi(t, m[2])
		if lines+remaining != budgetDirLimit {
			t.Errorf("散文の「%s 行・残り %s 行」は合計 %d 行で、上限の %d 行と合わない: %s",
				m[1], m[2], lines+remaining, budgetDirLimit, sentence)
		}
		if !hasBudgetPair(rows, lines, remaining) {
			t.Errorf("散文が現在形で主張する %d 行・残り %d 行を持つディレクトリが行数表に無い"+
				"（古いなら実測へ直し、当時の値なら同じ文に「当時」か「時点では」を入れる）: %s",
				lines, remaining, sentence)
		}
	}
	for _, m := range proseBudgetRemainder.FindAllStringSubmatch(sentence, -1) {
		claims++
		remaining := atoi(t, m[1])
		if !hasBudgetRemainder(rows, remaining) {
			t.Errorf("散文が現在形で主張する残り %d 行を持つディレクトリが行数表に無い"+
				"（古いなら実測へ直し、当時の値なら同じ文に「当時」か「時点では」を入れる）: %s",
				remaining, sentence)
		}
	}
	return claims
}

// budgetProseSentences は本書の散文を文に切って返す。
//
// 2 つの行数表と `## 改訂履歴` 以降は落とす——表は別のテストが実測と突き合わせており、
// 改訂履歴は当時の値だけを残す欄だからである。見出しは残す（残りを書いた見出しがある）。
func budgetProseSentences(t *testing.T) []string {
	t.Helper()

	body, _, _ := strings.Cut(readAtomicDesign(t), budgetRevisionHead)

	var prose strings.Builder
	lines := strings.Split(body, "\n")
	for i := 0; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) != budgetTableHeader {
			prose.WriteString(lines[i])
			prose.WriteString("\n")
			continue
		}
		for i+1 < len(lines) && strings.HasPrefix(strings.TrimSpace(lines[i+1]), "|") {
			i++
		}
	}
	return strings.FieldsFunc(prose.String(), func(r rune) bool { return r == '。' || r == '\n' })
}

// hasBudgetPair は行数と残りの組を持つ行が表にあるかを返す。
func hasBudgetPair(rows []budgetRow, lines, remaining int) bool {
	for _, row := range rows {
		if row.lines == lines && row.remaining == remaining {
			return true
		}
	}
	return false
}

// hasBudgetRemainder は残りが一致する行が表にあるかを返す。
func hasBudgetRemainder(rows []budgetRow, remaining int) bool {
	for _, row := range rows {
		if row.remaining == remaining {
			return true
		}
	}
	return false
}
