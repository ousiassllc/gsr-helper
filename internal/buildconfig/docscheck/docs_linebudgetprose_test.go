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
	//
	// `残り` と数の間に助詞を許すのは `残りは 65 行` `残りが 65 行` を取り逃がさない
	// ためである。**これは言い回しの列挙ではない**——拾う形は「残り」と数の並びのままで、
	// その間に挟まる助詞を吸収しているだけだからである。言い回しの列挙は「その書き方が
	// 出てくるたびに規則を足す」ことになって収束しないが、助詞は閉じた集合である。
	//
	// 数の後ろの助数詞まで拾うのは偽陽性を避けるためである。本書は `残りが 1 桁の
	// ディレクトリ` のように、実測値ではなく性質を語る形でも `残り` を使う。Go の
	// regexp に先読みが無いので助数詞をキャプチャし、それが budgetProseUnits に
	// 挙げた助数詞のときだけ呼び出し側で落とす（`行` と助数詞なしだけが実測値である）。
	proseMeasuredRemainder = regexp.MustCompile(`残り\s*[はがもの]?\s*(-?\d+)\s*(行|桁|件|つ|本|枚|箇所)?`)
	// proseMeasuredLines は行数を拾う。**見ないのは `0 行` / `1 行` / `2 行` の 3 つ
	// だけである**——本書は `atom を並べた 1 行` `runner 1 行のセル列` `2 行で並び` の
	// ように「表の 1 行」「一覧の 1 行」の意味でこの 2 つを常用するからで、行数の
	// 主張ではない。`0 行` が入るのは式の形が `[3-9]` を要求する帰結だが、残りが 0 に
	// なる状態は実在しうるので、そこは規約で守るしかない。
	// **床をそれより上へ置くと最も危険な帯が素通りする**——予算の残りは逼迫した
	// ディレクトリほど小さく 1 桁まで下がるので、「移せるのは N 行まで」という
	// 指示の N も 1 桁になる。床を 3 桁に置いていた頃は 2 桁の帯がまるごと
	// 素通りしており（Issue #164 の実害の原文「移せるのは 68 行まで」がこれである）、
	// 2 桁へ下げてもなお 1 桁の帯が同じ形で残っていた。
	proseMeasuredLines = regexp.MustCompile(`([3-9]|\d{2,})\s*行`)
)

// budgetProseUnits は proseMeasuredRemainder が拾っても実測値ではない助数詞である。
// `残りが 1 桁` `残り 3 件` のように、行数ではなく個数や性質を語る形がここに入る。
var budgetProseUnits = map[string]bool{
	"桁": true, "件": true, "つ": true, "本": true, "枚": true, "箇所": true,
}

// budgetProseExcluded は `docs/ui/atomic-design.md` で検査から外す節の見出しである
// （見出し行そのままの綴り）。
//
// **一覧は文書ごとに持つ**——この一覧は本書専用で、他の対象文書の一覧は
// budgetProseDocs の各 excluded にある（理由は budgetProseDoc の doc コメント）。
//
// **挙げてあるのはどれも当時の値を記録する節である。** ただし確かめてあるのは
// **この検査が拾う形（`N 行` と `残り N`）が残っていないこと**だけである——一覧から
// 1 件ずつ外して検査を回し、落ちた行がすべて `当時` / `着手時点では` /
// `Issue #NNN の時点では` と読める過去の記録か、`A → B` の遷移の記録であることを
// 確かめた。**それ以外の形は検査が構造的に見ない**——個数（`N 個` / `N ファイル` の
// ように行以外の助数詞で数える値）と、数値を伴わない語形の主張（`余裕がある` /
// `潤沢ではない`）がそれで、**そこは一覧の大小に関わらず本書の規約でしか守れない**
// （「行数の予算」の「行数の実測値は表だけが持つ」が範囲を明記している）。
// 記録はその Issue に紐づく事実なので、現在の実測へ寄せると議論の根拠そのものが
// 消える。だから節ごと外す。
//
// **載せてよいのは過去の値が要る節だけで、一覧は小さいほどよい。** 載っている節は検査の
// 外にあり、そこだけは規約で守るしかないからである。現在の値を行数表や
// `go tool linterly check` の出力への参照へ寄せれば除外は要らなくなるので、そうなった
// 節は一覧から外すこと——「`ui/organism/table` の本体を分割しない判断」は現在の
// ファイル数と行数を落とした時点で外れた。
//
// 「一覧タブを 1 枚足せる余裕」も過去の値を記録する節である——見出しに「空け方」「判断」は
// 無いが、中身は `ui` 直下の 2〜5 周目の空け方をその Issue の値で記録している。
//
// **除外は入れ子では広がらない。** 外れるのは**この一覧に挙げた見出しの節だけ**で、
// 範囲はその見出しから**次の見出し（深さを問わない）まで**である。入れ子の小節は自分が
// この一覧に載っているときだけ外れる（「`ui/organism/table` の本体を分割しない判断」を
// 外した後もその配下の「空け方（Issue #65）」が外れたままなのがこの形である）。深さで
// 打ち切っていた頃は「たまたま入れ子になっている」というだけの理由で無関係な小節まで
// 黙って外れており（`internal/buildconfig` の 2 つの判断が `ui/page/config` の節の配下に
// あった）、一覧を読んでも除外範囲が再現できなかった。いまは**この一覧の見出しの集合が
// そのまま除外範囲である。**
var budgetProseExcluded = []string{
	"##### `internal/disk` から `pathguard` を切り出した判断（Issue #101）",
	"##### `internal/setup` のフィクスチャを `setuptest` へ出した判断（Issue #103）",
	"#### `ui/page/config` を `page/configmodal` へ分けた判断（Issue #12 / 実施は Issue #104）",
	"##### `ui/page` を警告帯へ入れた判断（Issue #155）",
	"##### Issue #159 の空け方——`ui/page` を警告帯から出し、`ui/page/runners` に余裕を作った",
	"##### `internal/buildconfig` を警告帯へ入れた判断（PR #153 の 2 周目レビュー）",
	"##### `internal/buildconfig` を 2 つに分けた判断（Issue #161）",
	"##### Issue #140 / #141 / #142 の判断——警告帯へ入れて回帰テストを足した",
	"##### Issue #147 の空け方——警告帯の 3 つを全部出した",
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
	"##### 2 周目（2034 → 1513 行。Issue #106）",
	"##### 空け方（Issue #65）",
}

// proseLine は検査対象の 1 行と、その本書での行番号である。
type proseLine struct {
	no   int
	text string
}

// `budgetProseDocs` が挙げる 2 文書の散文は、行数の実測値を数値で持ってはならない（`残り N` と
// `N 行` が現れない）。免除と除外の詳細は docs/ui/atomic-design.md の「行数の実測値は表だけが
// 持つ」が持つ。
//
// 実測値は本書の 3 か所（2 つの行数表・散文・改訂履歴）へ手で写されており、表だけを
// 実測へ合わせても散文は古い数を現在形で語り続ける（Issue #164）。以前はこれを
// 「行数を主張する言い回し」を並べて拾い、表と突き合わせていたが、**言い回しの列挙は
// 収束しなかった**——レビューのたびに未カバーの書き方が見つかり、その都度検査を足す
// ことになった。そこで**言い回しを一切見ず、数値が無いことだけを見る**形に変えた。
// 現在の行数を数で持ってよいのは行数表だけで、散文は表を参照する。
//
// 除外は文書ごとの明示の一覧（budgetProseDoc.excluded＝過去の値を記録する節）と、行数表・コードブロック・
// 改訂履歴である。対象は budgetProseDocs に登録した文書すべてである。
func TestLineBudgetProseHasNoMeasuredNumbers(t *testing.T) {
	for _, doc := range budgetProseDocs {
		for _, line := range budgetProseLines(t, doc) {
			for _, m := range proseMeasuredRemainder.FindAllStringSubmatch(line.text, -1) {
				if budgetProseUnits[m[2]] {
					continue
				}
				t.Errorf("%s:%d: 散文が残りを数で書いている（%q）"+
					"——現在の値は行数表だけが持つ。散文は表を参照すること: %s",
					doc.name, line.no, m[0], line.text)
			}
			for _, m := range proseMeasuredLines.FindAllStringSubmatch(line.text, -1) {
				if budgetProseRuleConstants[m[1]] {
					continue
				}
				t.Errorf("%s:%d: 散文が行数を数で書いている（%q）"+
					"——現在の値は行数表だけが持つ。散文は表を参照すること: %s",
					doc.name, line.no, m[0], line.text)
			}
		}
	}
}

// budgetProseLines は doc から除外を落とした後の散文を、行番号付きで返す。
func budgetProseLines(t *testing.T, doc budgetProseDoc) []proseLine {
	t.Helper()

	lines := strings.Split(doc.read(t), "\n")
	code := fencedCodeLines(t, doc, lines)
	skip := append([]bool(nil), code...)
	skipBudgetTables(t, doc, lines, code, skip)
	skipRevisionHistory(t, doc, lines, code, skip)
	skipExcludedSections(t, doc, lines, code, skip)

	prose := make([]proseLine, 0, len(lines))
	for i, line := range lines {
		if skip[i] || strings.TrimSpace(line) == "" {
			continue
		}
		prose = append(prose, proseLine{no: i + 1, text: line})
	}
	// 除外の広がりすぎで検査対象が消えたまま緑になるのを防ぐ。
	if len(prose) == 0 {
		t.Fatalf("%s は除外を落とすと散文が 1 行も残らない（除外が広すぎる）", doc.name)
	}
	return prose
}

// fencedCodeLines は ``` で囲まれた範囲（囲みの行を含む）に印を付ける。
//
// 閉じ忘れは `t.Fatal` する。囲みが 1 つ足りないと、そこから文書の末尾までが黙って
// 検査対象から消え、**検査は何も見ないまま緑になる**——他の 4 つの保証（行数表が
// 期待した数だけある / `## 改訂履歴` がある / 一覧の見出しがある / 散文が残る）と
// 同じく、除外が現実と食い違ったことを声を上げて知らせる。
func fencedCodeLines(t *testing.T, doc budgetProseDoc, lines []string) []bool {
	t.Helper()

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
	if inCode {
		t.Fatalf("%s のコードブロックが閉じていない（``` の数が奇数）"+
			"——閉じ忘れた先が黙って検査対象から外れる", doc.name)
	}
	return code
}

// skipBudgetTables は行数表を外す。表は TestLineBudgetTablesMatchLinterly が
// 実測と突き合わせており、そこだけが現在の行数を数で持ってよい場所である。
//
// 位置は行番号ではなく表の見出し行で引く（parseBudgetTables と同じ引き方）。
// 期待する数は文書ごとに違う（budgetProseDoc.tables）ので、一致しなければ落とす——
// 表を持たない文書（tables が 0）で表が現れた場合も同じく落ちる。
func skipBudgetTables(t *testing.T, doc budgetProseDoc, lines []string, code, skip []bool) {
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
	if tables != doc.tables {
		t.Fatalf("%s の行数表が %d 個見つかった（%d 個のはず）", doc.name, tables, doc.tables)
	}
}

// skipRevisionHistory は `## 改訂履歴` 以降を外す。過去欄は当時の値を持つのが正しい。
//
// 見出しは**行頭でだけ**効かせる。字下げや文中の引用で切ると、そこから先が
// まるごと検査から外れる（改訂 1.96 の 2 周目レビューが踏んだ欠陥である）。
func skipRevisionHistory(t *testing.T, doc budgetProseDoc, lines []string, code, skip []bool) {
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
	t.Fatalf("%s に行頭の %q が無い", doc.name, budgetRevisionHead)
}

// skipExcludedSections は doc.excluded に挙げた見出しの節だけを外す。
//
// 打ち切りは**深さを見ない**——次の見出しが現れたらそこで止める。入れ子の小節を
// 外したいときは、その小節も同じ一覧へ書くこと（budgetProseExcluded の
// 「除外は入れ子では広がらない」を参照）。
func skipExcludedSections(t *testing.T, doc budgetProseDoc, lines []string, code, skip []bool) {
	t.Helper()

	for _, head := range doc.excluded {
		start := -1
		for i, line := range lines {
			if !code[i] && line == head {
				start = i
				break
			}
		}
		// 見出しの改名で除外が黙って外れる（あるいは効かなくなる）のを防ぐ。
		if start < 0 {
			t.Fatalf("除外に挙げた見出し %q が %s に無い", head, doc.name)
		}
		for i := start; i < len(lines); i++ {
			if i > start && !code[i] && isHeading(lines[i]) {
				break
			}
			skip[i] = true
		}
	}
}

// isHeading は行が ATX 見出し（行頭の `#` の並びと空白）かどうかを返す。
func isHeading(line string) bool {
	n := len(line) - len(strings.TrimLeft(line, "#"))
	return n > 0 && strings.HasPrefix(line[n:], " ")
}
