package docscheck

import (
	"regexp"
	"strings"
	"testing"
)

// budgetProseRuleConstants は散文に数値のまま書いてよい 3 桁以上の数である。
//
// 4 つとも `.linterly.yml` の上限と警告帯の境界であって、実測値ではない——300 は
// 1 ファイルの上限、330 はその 110% にあたるファイル側の ERROR 境界、2000 は
// 1 ディレクトリの上限、2200 は同じくディレクトリ側の ERROR 境界である。
// 規約の定数は数で書かれていないと読めないので、この 4 つだけを除く。
var budgetProseRuleConstants = map[string]bool{
	"300": true, "330": true, "2000": true, "2200": true,
}

var (
	// proseMeasuredRemainder は残りの主張を拾う。**残りは常に実測値である**——
	// 上限にも警告帯の境界にも「残り」は無いので、除外すべき定数が無い。
	proseMeasuredRemainder = regexp.MustCompile(`残り\s*-?\d+`)
	// proseMeasuredLines は 3 桁以上の行数を拾う。2 桁以下を見ないのは、本書が
	// `atom を並べた 1 行` `runner 1 行のセル列` のように「表の 1 行」「一覧の 1 行」
	// の意味で `1 行` を常用するからである（行数の主張ではない）。小さい残りは
	// 桁に関わらず proseMeasuredRemainder が拾う。
	proseMeasuredLines = regexp.MustCompile(`(\d{3,})\s*行`)
)

// budgetProseExcluded は検査から外す節の見出しである（見出し行そのままの綴り）。
//
// **どれも当時の値を記録する節である。** 空け方・判断の記録はその Issue に紐づく
// 事実なので、現在の実測へ寄せると議論の根拠そのものが消える。だから節ごと外し、
// 中身は本節の規約（「行数の予算」の「行数は表だけが数で持つ」）で守る。
//
// 「一覧タブを 1 枚足せる余裕」も同じ性格の節である——見出しに「空け方」「判断」は
// 無いが、中身は `ui` 直下の 2〜5 周目の空け方をその Issue の値で記録している。
//
// 外すのは見出しから同じ深さ以上の次の見出しまでで、入れ子の小節も一緒に外れる。
var budgetProseExcluded = []string{
	"##### `internal/disk` から `pathguard` を切り出した判断（Issue #101）",
	"##### `internal/setup` のフィクスチャを `setuptest` へ出した判断（Issue #103）",
	"#### `ui/page/config` を `page/configmodal` へ分けた判断（Issue #12 / 実施は Issue #104）",
	"#### `ui/page/disk` を `page/diskclean` へ分けた判断（Issue #13 / 実施は Issue #102）",
	"#### `ui/page/setup` を `page/setupmodal` へ分けた判断（Issue #8 の 2 周目 / 実施は Issue #105）",
	"##### 6 周目の空け方（Issue #128）",
	"##### 7 周目の判断（Issue #132）——切り出さずに警告帯へ入った",
	"##### 8 周目の判断（Issue #136）——警告帯のまま回帰テストを足した",
	"##### 9 周目の空け方（Issue #139）——警告帯から出た",
	"##### 10 周目の空け方（Issue #148）——`background.go` を `ui/startup` へ出した",
	"#### 一覧タブを 1 枚足せる余裕（Issue #35 / 実績は Issue #11）",
	"#### サービス制御を `page/runnerop` へ出した判断（Issue #5）",
	"#### `ui` 直下を分割した判断（2194 → 1845 行）",
	"#### `ui/molecule` を分割した判断（1 周目 1764 → 1197 行 / 2 周目 2034 → 1513 行）",
	"#### `ui/organism/table` の本体を分割しない判断",
}

// proseLine は検査対象の 1 行と、その本書での行番号である。
type proseLine struct {
	no   int
	text string
}

// 散文は行数の実測値を数値で持ってはならない。
//
// 実測値は本書の 3 か所（2 つの行数表・散文・改訂履歴）へ手で写されており、表だけを
// 実測へ合わせても散文は古い数を現在形で語り続ける（Issue #164）。以前はこれを
// 「行数を主張する言い回し」を並べて拾い、表と突き合わせていたが、**言い回しの列挙は
// 収束しなかった**——レビューのたびに未カバーの書き方が見つかり、その都度検査を足す
// ことになった。そこで**言い回しを一切見ず、数値が無いことだけを見る**形に変えた。
// 現在の行数を数で持ってよいのは行数表だけで、散文は表を参照する。
//
// 除外は budgetProseExcluded の明示の一覧と、行数表・コードブロック・改訂履歴である。
func TestLineBudgetProseHasNoMeasuredNumbers(t *testing.T) {
	for _, line := range budgetProseLines(t) {
		if m := proseMeasuredRemainder.FindString(line.text); m != "" {
			t.Errorf("atomic-design.md:%d: 散文が残りを数で書いている（%q）"+
				"——現在の値は行数表だけが持つ。散文は表を参照すること: %s", line.no, m, line.text)
		}
		for _, m := range proseMeasuredLines.FindAllStringSubmatch(line.text, -1) {
			if budgetProseRuleConstants[m[1]] {
				continue
			}
			t.Errorf("atomic-design.md:%d: 散文が行数を数で書いている（%q）"+
				"——現在の値は行数表だけが持つ。散文は表を参照すること: %s", line.no, m[0], line.text)
		}
	}
}

// budgetProseLines は除外を落とした後の散文を、行番号付きで返す。
func budgetProseLines(t *testing.T) []proseLine {
	t.Helper()

	lines := strings.Split(readAtomicDesign(t), "\n")
	code := fencedCodeLines(lines)
	skip := append([]bool(nil), code...)
	skipBudgetTables(t, lines, code, skip)
	skipRevisionHistory(t, lines, code, skip)
	skipExcludedSections(t, lines, code, skip)

	prose := make([]proseLine, 0, len(lines))
	for i, line := range lines {
		if skip[i] || strings.TrimSpace(line) == "" {
			continue
		}
		prose = append(prose, proseLine{no: i + 1, text: line})
	}
	// 除外の広がりすぎで検査対象が消えたまま緑になるのを防ぐ。
	if len(prose) == 0 {
		t.Fatal("除外を落とすと散文が 1 行も残らない（除外が広すぎる）")
	}
	return prose
}

// fencedCodeLines は ``` で囲まれた範囲（囲みの行を含む）に印を付ける。
func fencedCodeLines(lines []string) []bool {
	code := make([]bool, len(lines))
	inCode := false
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			code[i] = true
			inCode = !inCode
			continue
		}
		code[i] = inCode
	}
	return code
}

// skipBudgetTables は 2 つの行数表を外す。表は TestLineBudgetTablesMatchLinterly が
// 実測と突き合わせており、そこだけが現在の行数を数で持ってよい場所である。
//
// 位置は行番号ではなく表の見出し行で引く（parseBudgetTables と同じ引き方）。
func skipBudgetTables(t *testing.T, lines []string, code, skip []bool) {
	t.Helper()

	var tables int
	for i, line := range lines {
		if code[i] || strings.TrimSpace(line) != budgetTableHeader {
			continue
		}
		tables++
		for ; i < len(lines) && strings.HasPrefix(strings.TrimSpace(lines[i]), "|"); i++ {
			skip[i] = true
		}
	}
	// 表の見出しが変わって除外が効かなくなった／表が消えたことを声を上げて知らせる。
	if tables != 2 {
		t.Fatalf("行数表が %d 個しか見つからない（UI 層と UI 層の外の 2 つのはず）", tables)
	}
}

// skipRevisionHistory は `## 改訂履歴` 以降を外す。過去欄は当時の値を持つのが正しい。
//
// 見出しは**行頭でだけ**効かせる。字下げや文中の引用で切ると、そこから先が
// まるごと検査から外れる（改訂 1.96 の 2 周目レビューが踏んだ欠陥である）。
func skipRevisionHistory(t *testing.T, lines []string, code, skip []bool) {
	t.Helper()

	for i, line := range lines {
		if code[i] || line != budgetRevisionHead {
			continue
		}
		for ; i < len(lines); i++ {
			skip[i] = true
		}
		return
	}
	t.Fatalf("atomic-design.md に行頭の %q が無い", budgetRevisionHead)
}

// skipExcludedSections は budgetProseExcluded の節を、入れ子の小節ごと外す。
func skipExcludedSections(t *testing.T, lines []string, code, skip []bool) {
	t.Helper()

	for _, head := range budgetProseExcluded {
		start := -1
		for i, line := range lines {
			if !code[i] && line == head {
				start = i
				break
			}
		}
		// 見出しの改名で除外が黙って外れる（あるいは効かなくなる）のを防ぐ。
		if start < 0 {
			t.Fatalf("除外に挙げた見出し %q が atomic-design.md に無い", head)
		}
		level := headingLevel(head)
		for i := start; i < len(lines); i++ {
			if i > start && !code[i] {
				if l := headingLevel(lines[i]); 0 < l && l <= level {
					break
				}
			}
			skip[i] = true
		}
	}
}

// headingLevel は見出し行の `#` の数を返す。見出しでなければ 0 を返す。
func headingLevel(line string) int {
	n := len(line) - len(strings.TrimLeft(line, "#"))
	if n == 0 || !strings.HasPrefix(line[n:], " ") {
		return 0
	}
	return n
}
