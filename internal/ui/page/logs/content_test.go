package logs

import (
	"fmt"
	"slices"
	"testing"

	dlogs "github.com/ousiassllc/gsr-helper/internal/logs"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/pagetest"
	"github.com/ousiassllc/gsr-helper/internal/ui/token"
)

// 本文の 2 つの入口（content.go の applyLines / pushLines）を突き合わせる。
//
// **1 本で縛るのが要点である。** 追従中に通るのは pushLines だけ、フィルタの確定・取消・
// 解除と対象の切り替え・前面への復帰で通るのは applyLines だけなので、経路ごとに別々の
// 検証を置くと片方だけ壊れても緑になる。実際、applyLines の装飾・pushLine の絞り込み・
// キャッシュの追い出しはどれを外しても他のテストは全件緑だった。同じ行から同じ本文が
// できることを見れば、その 3 つがまとめて塞がる。
//
// **合わせて費用も数える。** 増分の入口の目的は結果ではなく費用であり、結果だけを見ると
// 全再構築へ書き戻しても緑のままになる（content.go の doc）。

// pushBatch は突き合わせで流す 1 束の行数。購読からは届いている行がまとめて渡る。
const pushBatch = 64

// warnLines は WARN の行を n 行返す。半分は絞り込み（`k`）に掛からない文言にする。
//
// すべて WARN にするのは装飾の回数を数えるためである。molecule.LogLine は素の行では
// Render を呼ばない（大半を占める行の 1 回を省く最適化）ので、素の行では数えられない。
func warnLines(n int) []dlogs.Line {
	out := make([]dlogs.Line, 0, n)
	for i := range n {
		text := fmt.Sprintf("drop %d", i)
		if i%2 == 0 {
			text = fmt.Sprintf("keep %d", i)
		}
		out = append(out, dlogs.Line{Text: text, Level: dlogs.LevelWarn})
	}
	return out
}

// filtered はフィルタ `k` を確定した Logs タブを返す。
func filtered(st page.StateMsg) Model {
	m := New(testTab, st)
	m.body.StartFilter()
	m.body, _ = m.body.Update(pagetest.Press("k"))
	m.body.AcceptFilter()
	return m
}

// 増分と全再構築は同じ本文を作り、増分は届いた行数ぶんしか働かない。
func TestPushLinesMatchesApplyLines(t *testing.T) {
	var styled int
	st := pagetest.State(80, 20)
	// 色ありの配色を使うのは、素通しの配色では装飾した行と素の行が同じ文字列になり、
	// 「装飾を捨てて素の行を積む」変異を見分けられないためである。
	st.Styles = token.NewStyles(true, true)
	st.Styles.Warn = st.Styles.Warn.Transform(func(s string) string { styled++; return s })

	// 上限を超える行数を流し、キャッシュの追い出しまで通す。
	in := warnLines(maxLines + pushBatch)
	inc := filtered(st)
	inc.pushLines(in[:1])
	compiled := inc.filterRe
	for i := 1; i < len(in); i += pushBatch {
		inc.pushLines(in[i:min(i+pushBatch, len(in))])
	}
	pushed := styled

	styled = 0
	full := filtered(st)
	full.lines = inc.lines
	full.applyLines()

	if !slices.Equal(inc.styled, full.styled) {
		t.Fatalf("増分と全再構築で本文が違う（増分 %d 行 / 全再構築 %d 行）",
			len(inc.styled), len(full.styled))
	}
	if len(inc.styled) == 0 || len(inc.styled) >= len(inc.lines) {
		t.Fatalf("絞り込みが効いていない（本文 %d 行 / 保持 %d 行）", len(inc.styled), len(inc.lines))
	}
	if want := len(in) / 2; pushed != want {
		t.Errorf("増分の装飾の回数 = %d, want %d（届いた行のうち絞り込みを通った数）", pushed, want)
	}
	if inc.filterRe != compiled {
		t.Error("フィルタが変わっていないのに正規表現を解き直している")
	}
}
