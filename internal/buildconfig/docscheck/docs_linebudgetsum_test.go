package docscheck

import (
	"regexp"
	"testing"
)

var (
	// proseMeasuredSum は `A + B + C = D` の形の式を拾う。
	//
	// **式を別に拾うのは各項が裸の数だからである。** 行数の実測値を拾う側
	// （proseMeasuredLines）が見るのは `N 行` という助数詞付きの形なので、
	// 合計の式で拾えるのは右辺の `D 行` だけであり、**左辺の各項は助数詞を
	// 持たないので最初から見えていない**。しかも右辺が `行` を伴わない書き方
	// （`合計は 1216 + 1334 + 44 = 2594 である`）になった瞬間、4 項とも
	// まるごと素通りする（Issue #167）。1 文で 4 つの実測値を主張する形なので、
	// 素通りすると写しが 1 か所ではなく 4 か所増える。
	//
	// `=` の後ろに `\**` を挟むのは、本書が右辺を bold で囲む形
	// （`= **2196 行**`）で書くためである。
	proseMeasuredSum = regexp.MustCompile(`\d+(?:\s*\+\s*\d+)+\s*=\s*\**\s*\d+`)
	// proseSumNumber は式に含まれる数を数えるためのものである。桁で切る判定
	// （proseSumIsMeasured）にだけ使う。
	proseSumNumber = regexp.MustCompile(`\d+`)
)

// budgetProseSumFloor は、式を行数の実測値と見なす桁数の下限である。
//
// **桁で切るのは言い回しの列挙ではない。** 行数の合計は 1 ディレクトリ 2000 行の
// 桁で数えるものである以上どうしても 3 桁以上になり、逆に**2 桁以下だけで閉じた式は
// 行数ではない算術**である——本書が書くのは列幅の見積もり（`6 + 66 + 3 = 75 セル`）で、
// これは 80 桁の端末に収まるかどうかの話であって行数ではない。除外すべき集合が
// このように閉じているので、**新しい書き方が出てくるたびに規則を足すことにはならない**
// （日本語の言い回しを並べる方式が収束しなかったのは、その集合が閉じていないためである）。
const budgetProseSumFloor = 3

// proseSumIsMeasured は式が行数の実測値を主張しているかどうかを返す。
//
// 式に含まれる数のうち 1 つでも budgetProseSumFloor 桁以上あれば実測値と見なす。
// 左辺の項が小さくても（`1216 + 1334 + 44`）どれか 1 つが 3 桁以上であれば
// 行数の合計だからである。
func proseSumIsMeasured(expr string) bool {
	for _, n := range proseSumNumber.FindAllString(expr, -1) {
		if len(n) >= budgetProseSumFloor {
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
// **だから式そのものを別に拾う。**
//
// 直す向きは検査を式の評価（各項を行数表と突き合わせ、和が右辺と一致することを
// 確かめる）へ広げる側ではなく、**式を落として行数表への参照へ一本化する側**である。
// 評価しても表と散文の一致が守られるだけで、**写しそのものは残る**からである。
//
// 除外は TestLineBudgetProseHasNoMeasuredNumbers と同じもの（budgetProseLines が
// 行数表・コードブロック・改訂履歴・budgetProseExcluded の節を落とす）を使う。
// 過去の値を記録する節は除外の中にあるので、そこでは分割の前後で合計がどう動いたかを
// 式で書いてよい——その Issue に紐づく事実だからである。
func TestLineBudgetProseHasNoSumExpressions(t *testing.T) {
	for _, line := range budgetProseLines(t) {
		for _, expr := range proseMeasuredSum.FindAllString(line.text, -1) {
			if !proseSumIsMeasured(expr) {
				continue
			}
			t.Errorf("atomic-design.md:%d: 散文が行数の合計を式で書いている（%q）"+
				"——現在の値は行数表だけが持つ。散文は表を参照すること: %s", line.no, expr, line.text)
		}
	}
}

// 合計の式の検出は、行数の式だけを拾い、行数以外の算術は拾わない。
//
// **両方向を固定するために単体で持つ。** 拾えなくなれば Issue #167 の実害
// （4 項ぶんの実測値が素通りし、散文が古い合計を現在形で語り続ける）がそのまま戻る。
// 逆に拾いすぎれば、本書が正当に書く算術（列幅の見積もり）まで落ちるので、
// それを通すために budgetProseExcluded へ節を足すことになる——**一覧は小さいほどよい**
// というのが本書の規約なので、偽陽性は除外一覧を膨らませる形で規約を崩す。
func TestProseMeasuredSumMatchesOnlyLineTotals(t *testing.T) {
	measured := []string{
		// Issue #167 が挙げた原文。
		"現在の 3 つの合計は 1216 + 1334 + 44 = 2594 行である",
		// 右辺が bold で囲まれる形（本書の書き方）。
		"分けた当時は 2122 行が 1216 + 936 + 44 = **2196 行**（+74）になった",
		// 右辺が助数詞を持たない形。既存の `N 行` の検査で拾えないのがこれである。
		"合計は 1216 + 1334 + 44 = 2594 である",
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
		// 本書が実際に書いている列幅の見積もり。行数ではない算術である。
		"`LogColumns()` は判定上も 6 + 66 + 3 = 75 セルで 80 に収まる",
	}
	for _, text := range notMeasured {
		expr := proseMeasuredSum.FindString(text)
		if expr != "" && proseSumIsMeasured(expr) {
			t.Errorf("行数ではない算術を実測値として拾っている（%q）: %s", expr, text)
		}
	}
}
