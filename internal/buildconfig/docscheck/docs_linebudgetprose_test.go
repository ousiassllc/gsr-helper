package docscheck

import (
	"regexp"
	"strings"
	"testing"
)

// proseBudgetPair は散文の「N 行・残り M 行」「N 行（残り M 行」という主張を拾う。
var proseBudgetPair = regexp.MustCompile(`(\d+)\s*行(?:・|（)残り\s*(-?\d+)\s*行`)

// proseBudgetRemainders はディレクトリの残りだけを主張する書き方である。
//
// 1 つ目の末尾の `行` を必須にしてあるのは、`296 行（残り 4）` のようにファイル側の
// 予算（1 ファイル 300 行）を指す書き方をわざと外すためである。
//
// 2 つ目・3 つ目は次の Issue へ予算を渡す指示の形式で、拾った数はディレクトリの残り
// として読む。形式を 2 つに絞ってあるのは、`予算` と `N 行` が同じ句にあれば拾う実装が
// 誤検出するからである——「1 ファイル 300 行の予算」は 1 ファイルの上限であり、
// 「当時の予算は『ERROR 境界 2200 まで 95 行』」は境界までの距離であって、どちらも
// ディレクトリの残りではない。
var proseBudgetRemainders = []*regexp.Regexp{
	regexp.MustCompile(`残り(?:は)?\s*(-?\d+)\s*行`),
	regexp.MustCompile(`(-?\d+)\s*行を予算として読む`),
	regexp.MustCompile(`移せるのは\s*(-?\d+)\s*行まで`),
}

// 散文が現在形で主張する行数・残りも、行数表と一致していなければならない。
//
// 実測値は表・散文・改訂履歴の 3 か所に写されているのに、これまで検査されていたのは
// どこでもなかった。表だけを実測に合わせても、散文は古い数を現在形で語り続ける——
// 実際、散文が後続の Issue に示した予算は本当の値より 3 行多かった（Issue #164）。
//
// **過去の値を書くときは同じ句に `当時` か `時点では` を入れること。** それがこの
// 検査の唯一の除外であり、規約は本書の「行数表と散文は実測に合わせる」に書いてある。
func TestLineBudgetProseMatchesTables(t *testing.T) {
	uiRows, nonUIRows := parseBudgetTables(t)
	rows := append(append(make([]budgetRow, 0, len(uiRows)+len(nonUIRows)), uiRows...), nonUIRows...)
	idx := newBudgetRowIndex(uiRows, nonUIRows)

	var claims int
	for _, sentence := range budgetProseSentences(t) {
		claims += checkProseBudgetClaims(t, sentence, rows, idx)
	}
	// 表の切り落としが文書ごと食い潰しても緑になるのを防ぐ。
	if claims == 0 {
		t.Fatal("散文から行数の主張を 1 件も拾えない（表の切り落としが広すぎる可能性がある）")
	}
}

// checkProseBudgetClaims は 1 文の中の行数の主張を表と突き合わせ、拾った件数を返す。
//
// ディレクトリの対応付けは文単位、過去の印（`当時`）の判定は句単位で行う。文の前半で
// ディレクトリを名指しし、後半で数を書く文体を壊さずに、同じ文で過去と現在を対比する
// 書き方のうち現在形の句だけを検査するための配置である。
func checkProseBudgetClaims(t *testing.T, sentence string, rows []budgetRow, idx budgetRowIndex) int {
	t.Helper()

	// 文が名指しするディレクトリがあれば、その行とだけ突き合わせる。表のどこかに
	// 同じ数があればよい形だと、別ディレクトリの値と偶然一致した陳腐化した主張が
	// 素通りする（`ui/page` の「1998 行・残り 2 行」が `internal/logs` と一致していた）。
	scope, where := rows, "行数表"
	if named := idx.namedBudgetRows(sentence); len(named) > 0 {
		scope, where = named, "文が名指しする "+budgetRowsLabel(named)
	}

	var claims int
	for _, clause := range strings.Split(sentence, "、") {
		// 過去の値は当時の Issue に紐づく記録なので、現在の実測へ寄せてはならない。
		// 除外は句どまりにする——過去と現在を 1 文で対比するのが本書の文体であり、
		// 文ごと外すとこの検査が最も守るべき現在形の主張がちょうど抜ける。
		if strings.Contains(clause, "当時") || strings.Contains(clause, "時点では") {
			continue
		}
		claims += checkProseBudgetClause(t, clause, sentence, scope, where)
	}
	return claims
}

// checkProseBudgetClause は 1 句の主張を突き合わせ、拾った件数を返す。
//
// 句の区切りは `、` だけである（文はすでに `。` と改行で切れている）。`（` `）` では
// 割らない——`proseBudgetPair` は `1998 行（残り 2 行` のように括弧をまたいで組を拾う
// ので、そこで割ると組の主張が残りだけの主張へ落ちて検査が弱くなる。
func checkProseBudgetClause(t *testing.T, clause, sentence string, scope []budgetRow, where string) int {
	t.Helper()

	var claims int
	for _, m := range proseBudgetPair.FindAllStringSubmatch(clause, -1) {
		claims++
		lines, remaining := atoi(t, m[1]), atoi(t, m[2])
		if lines+remaining != budgetDirLimit {
			t.Errorf("散文の「%s 行・残り %s 行」は合計 %d 行で、上限の %d 行と合わない: %s",
				m[1], m[2], lines+remaining, budgetDirLimit, sentence)
		}
		if !hasBudgetPair(scope, lines, remaining) {
			t.Errorf("散文が現在形で主張する %d 行・残り %d 行が %s に無い"+
				"（古いなら実測へ直し、当時の値なら同じ句に「当時」か「時点では」を入れる）: %s",
				lines, remaining, where, sentence)
		}
	}
	for _, re := range proseBudgetRemainders {
		for _, m := range re.FindAllStringSubmatch(clause, -1) {
			claims++
			remaining := atoi(t, m[1])
			if !hasBudgetRemainder(scope, remaining) {
				t.Errorf("散文が現在形で主張する残り %d 行が %s に無い"+
					"（古いなら実測へ直し、当時の値なら同じ句に「当時」か「時点では」を入れる）: %s",
					remaining, where, sentence)
			}
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

	var prose strings.Builder
	lines := strings.Split(readAtomicDesign(t), "\n")
	for i := 0; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		// 見出しは行頭でだけ効かせる。文字列として探すと、散文がこの見出しを
		// コード引用として書いただけでそれ以降がまるごと検査から外れる。
		if line == budgetRevisionHead {
			break
		}
		if line != budgetTableHeader {
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

// hasBudgetPair は行数と残りの組を持つ行があるかを返す。
func hasBudgetPair(rows []budgetRow, lines, remaining int) bool {
	for _, row := range rows {
		if row.lines == lines && row.remaining == remaining {
			return true
		}
	}
	return false
}

// hasBudgetRemainder は残りが一致する行があるかを返す。
func hasBudgetRemainder(rows []budgetRow, remaining int) bool {
	for _, row := range rows {
		if row.remaining == remaining {
			return true
		}
	}
	return false
}
