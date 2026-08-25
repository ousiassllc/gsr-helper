package docscheck

import (
	"regexp"
	"slices"
	"strconv"
	"testing"
)

var (
	// proseMeasuredSum は数が演算子で連なった式を拾う。`= D` の右辺は任意である
	// （なぜ式を別に拾うかは TestLineBudgetProseHasNoSumExpressions の doc コメント）。
	//
	// **必須なのは「2 つ以上の数が演算子で連なっていること」だけで、`= D` は任意**である。
	// `=` を要求していた頃は `内訳は 1216 + 1334 + 44 で、合わせて 2594 になる` が
	// 素通りしており、これは Issue #167 が挙げた実害（1 文で 4 つの実測値を主張する）
	// そのままの形だった。全角（`＋` `−` `＝`）も同じ式なので同じ文字クラスに入れる
	// ——**演算子は閉じた集合なので、これは言い回しの列挙には当たらない。**
	//
	// **`-` だけは前後の空白を必須にする。** 空白なしを許すと ISO 形式の日付
	// （`2026-08-25`）が式として拾われ、改訂履歴の外にある日付までが偽陽性になる。
	// 空白の有無で閉じるのも記号の形の話であって、言い回しの列挙ではない。
	// `=` の後ろの `\**` は、本書が右辺を bold で囲む形（`= **2196 行**`）で書くため。
	proseMeasuredSum = regexp.MustCompile(
		`\d+(?:(?:\s*[+＋]\s*|\s+[-−]\s+)\d+)+(?:\s*[=＝]\s*\**\s*\d+)?`)
	// proseSumNumber は式に含まれる数を数える。値で切る判定にだけ使う。
	proseSumNumber = regexp.MustCompile(`\d+`)
)

// budgetProseSumFloor は、式に含まれる数を行数の実測値と見なす値の下限である。
//
// **床は proseMeasuredLines とそろえてある**——あちらが `N 行` を拾うのは `N` が 3 以上の
// ときなので（見ないのは `0 行` / `1 行` / `2 行` の 3 つだけ）、式の側だけを 3 桁で切ると
// 同じスイートの中で床が食い違う。ファイル単位や小さいディレクトリの行数は 2 桁になりうる
// （`internal/buildconfig/buildconfigtest` は 2 桁である）ので、3 桁の床では
// `内訳は 44 + 30 = 74 である` がまるごと素通りしていた。**値で切るのは言い回しの列挙では
// ない。** ただし床をここまで下げると行数ではない算術（列幅の見積もり）も実測値の側に入る。
// 正規表現で除こうとすると言い回しの列挙に戻るので、**除外は budgetProseDoc.sumExcluded の
// 明示の見出し一覧**で持つ。
const budgetProseSumFloor = 3

// proseSumIsMeasured は式が行数の実測値を主張しているかどうかを返す。
//
// 式に含まれる数が**すべて 2 以下のときだけ**実測値でないと見なす。左辺の項が小さくても
// （`1216 + 1334 + 44`）どれか 1 つが 3 以上であれば行数の合計だからである。
func proseSumIsMeasured(expr string) bool {
	for _, n := range proseSumNumber.FindAllString(expr, -1) {
		// 桁あふれは行数ではありえない大きさなので実測値の側へ倒す。
		if v, err := strconv.Atoi(n); err != nil || v >= budgetProseSumFloor {
			return true
		}
	}
	return false
}

// 散文は行数の合計を式で書いてはならない。
//
// `A + B + C = D 行` は 1 文で 4 つの実測値を主張するので、写しが 1 か所ではなく
// 4 か所増える。にもかかわらず TestLineBudgetProseHasNoMeasuredNumbers が拾えるのは
// 右辺の `D 行` だけで、右辺が助数詞を失えば 4 項とも素通りする（Issue #167）。
// **だから式そのものを別に拾う。** 直す向きは検査を式の評価へ広げる側ではなく、
// **式を落として行数表への参照へ一本化する側**である——評価しても表と散文の一致が
// 守られるだけで、**写しそのものは残る**からである。
//
// 対象は TestLineBudgetProseHasNoMeasuredNumbers と同じ budgetProseDocs で、除外は
// **doc.excluded ∪ doc.sumExcluded**（行数表・コードブロック・改訂履歴に加えて、過去の値を
// 記録する節と、行数と無関係な算術の節）である。過去の値を記録する節では分割の前後で
// 合計がどう動いたかを式で書いてよい——その Issue に紐づく事実だからである。
func TestLineBudgetProseHasNoSumExpressions(t *testing.T) {
	for _, doc := range budgetProseDocs {
		for _, line := range budgetProseLines(t, doc.forSumCheck()) {
			for _, expr := range proseMeasuredSum.FindAllString(line.text, -1) {
				if !proseSumIsMeasured(expr) {
					continue
				}
				t.Errorf("%s:%d: 散文が行数の合計を式で書いている（%q）"+
					"——現在の値は行数表だけが持つ。散文は表を参照すること: %s",
					doc.name, line.no, expr, line.text)
			}
		}
	}
}

// 合計の式の検出は、行数の式だけを拾い、行数以外の算術は拾わない。
//
// **両方向を固定するために単体で持つ。** 拾えなくなれば Issue #167 の実害（4 項ぶんの
// 実測値が素通りし、散文が古い合計を現在形で語り続ける）がそのまま戻る。逆に拾いすぎれば、
// 本書が正当に書く算術を通すために sumExcluded へ節を足すことになる——**一覧は小さいほど
// よい**というのが本書の規約なので、偽陽性は除外一覧を膨らませる形で規約を崩す。
func TestProseMeasuredSumMatchesOnlyLineTotals(t *testing.T) {
	measured := []string{
		// Issue #167 が挙げた原文。
		"現在の 3 つの合計は 1216 + 1334 + 44 = 2594 行である",
		// 右辺が bold で囲まれる形（本書の書き方）。
		"分けた当時は 2122 行が 1216 + 936 + 44 = **2196 行**（+74）になった",
		// 右辺が助数詞を持たない形。既存の `N 行` の検査で拾えないのがこれである。
		"合計は 1216 + 1334 + 44 = 2594 である",
		// 以下 3 つは実測で素通りを再現した形である——(1) 2 桁で閉じた行数（3 桁の床では
		// 素通りしていた）、(2) `=` を持たず `+` の連なりだけの形（Issue #167 の実害
		// そのままである）、(3) 全角の演算子。
		"内訳は 44 + 30 = 74 である。",
		"内訳は 1216 + 1334 + 44 で、合わせて 2594 になる。",
		"合計は 1216 ＋ 1334 ＋ 44 ＝ 2594 である。",
		// 差の形。空白を伴う `-` は式の演算子である。
		"内訳は 2000 - 1906 = 94 である。",
	}
	for _, text := range measured {
		expr := proseMeasuredSum.FindString(text)
		if expr == "" {
			t.Errorf("行数の合計の式を拾えていない: %s", text)
			continue
		}
		if !proseSumIsMeasured(expr) {
			t.Errorf("拾った式が実測値と見なされていない（%q）: %s", expr, text)
		}
	}

	notMeasured := []string{
		// すべての数が 2 以下の式。行数ではありえない。
		"チェックボックスは 1 + 1 = 2 セルである",
		// ISO 形式の日付。`-` の前後に空白を要求しているので式として拾わない。
		"改訂 1.99 は 2026-08-25 に入った",
	}
	for _, text := range notMeasured {
		expr := proseMeasuredSum.FindString(text)
		if expr != "" && proseSumIsMeasured(expr) {
			t.Errorf("行数ではない算術を実測値として拾っている（%q）: %s", expr, text)
		}
	}

	// **列幅の見積もり（`6 + 66 + 3 = 75 セル`）は床では落ちない**——66 も 75 も 3 以上で、
	// 床を proseMeasuredLines とそろえた以上そうなる。式の形で除き分けようとすると
	// 言い回しの列挙に戻るので、**除外は sumExcluded の明示の見出し一覧が持つ**。ここでは
	// その節が一覧に載っていることを固定する（節が消えれば skipExcludedSections が落ちる）。
	if !slices.Contains(budgetProseSumExcluded, budgetProseWidthSection) {
		t.Errorf("atomic-design.md の sumExcluded に %q が無い（列幅の見積もりが偽陽性になる）",
			budgetProseWidthSection)
	}
}
