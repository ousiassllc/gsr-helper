package molecule

import (
	"reflect"
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/ui/token"
)

// columnSets は列を持つ一覧をすべて返す。
func columnSets() map[string][]token.Column {
	return map[string][]token.Column{
		"RunnerColumns": token.RunnerColumns(),
		"OrphanColumns": token.OrphanColumns(),
		"JobColumns":    token.JobColumns(),
	}
}

// 表示を保証する幅（80）では 1 列も落とさず、それ以上の幅でも溢れない。
//
// 非機能要件の「最小端末サイズは 80x24。これを下回る場合は列を省略し」の 80 は
// 保証する側であり、省略が始まるのはそれより狭いときである。孤児ユニットの列は
// ColumnDropOrder に 1 つも含まれないため、末尾から落とす経路がここで効く。
func TestColumnsFitFromMinToWideWidth(t *testing.T) {
	for name, all := range columnSets() {
		if got, want := columnIDs(Columns(all, token.WidthTarget)), columnIDs(all); !reflect.DeepEqual(got, want) {
			t.Errorf("%s: 幅 %d の列 = %v, want %v", name, token.WidthTarget, got, want)
		}
		for width := 120; width >= token.WidthMin; width-- {
			if cols := Columns(all, width); !columnsFit(cols, width) {
				t.Errorf("%s: 幅 %d で選んだ列 %v が収まらない", name, width, columnIDs(cols))
			}
		}
	}
}

// 幅を狭めると _WORK → VERSION → MANAGED → SCOPE の順に列が落ちる。
func TestColumnsDropOrderByWidth(t *testing.T) {
	cases := []struct {
		width int
		want  []string
	}{
		{100, []string{"NAME", "SCOPE", "MANAGED", "SVC", "JOB", "VERSION", "_WORK"}},
		{80, []string{"NAME", "SCOPE", "MANAGED", "SVC", "JOB", "VERSION", "_WORK"}},
		{78, []string{"NAME", "SCOPE", "MANAGED", "SVC", "JOB", "VERSION"}},
		{71, []string{"NAME", "SCOPE", "MANAGED", "SVC", "JOB"}},
		{61, []string{"NAME", "SCOPE", "SVC", "JOB"}},
		{53, []string{"NAME", "SVC", "JOB"}},
	}
	for _, c := range cases {
		if got := columnIDs(Columns(token.RunnerColumns(), c.width)); !reflect.DeepEqual(got, c.want) {
			t.Errorf("幅 %d の列 = %v, want %v", c.width, got, c.want)
		}
	}
}

// 幅を 1 刻みで狭めても、落ちる順が飛ばず逆転しない。
//
// 落とす順（screens.md の「端末幅による列の省略」）は受け入れ条件そのものなので、
// 境界の 1 セルずれを個別の期待値ではなく走査で確かめる。
func TestColumnsDropOneByOneAsWidthShrinks(t *testing.T) {
	order := token.ColumnDropOrder()
	prev := columnIDs(Columns(token.RunnerColumns(), 120))
	dropped := 0

	for width := 119; width >= token.WidthMin; width-- {
		got := columnIDs(Columns(token.RunnerColumns(), width))
		switch len(got) {
		case len(prev):
			if !reflect.DeepEqual(got, prev) {
				t.Errorf("幅 %d: 列数が同じなのに内容が変わった %v → %v", width, prev, got)
			}
		case len(prev) - 1:
			if dropped >= len(order) {
				t.Fatalf("幅 %d: 落とす順を超えて列が落ちた（%v）", width, got)
			}
			if containsID(got, order[dropped]) {
				t.Errorf("幅 %d: 落ちるべき列 %s が残っている（%v）", width, order[dropped], got)
			}
			dropped++
		default:
			t.Fatalf("幅 %d: 列数が %d から %d へ飛んだ（%v）", width, len(prev), len(got), got)
		}
		prev = got
	}
}

// 常に表示する列は幅が足りなくても落ちない。落とし切ってもそれだけは残る。
//
// 同じ幅で結果が変わらないこと（map の走査順に依存した実装などの非決定性）と、
// 返り値を書き換えても次の呼び出しに影響しないことも併せて確かめる。実装と実装を
// 比べる形になるが、非決定性は期待値を別に書けない。
func TestColumnsKeepAlwaysColumns(t *testing.T) {
	for _, width := range []int{200, 100, 80, 72, 66, 60, 59, 30, 1, 0, -10} {
		got := columnIDs(Columns(token.RunnerColumns(), width))
		for _, id := range token.ColumnsAlways() {
			if !containsID(got, id) {
				t.Errorf("幅 %d で常に表示する列 %s が落ちた（%v）", width, id, got)
			}
		}
	}
	if got, want := columnIDs(Columns(token.RunnerColumns(), 30)), token.ColumnsAlways(); !reflect.DeepEqual(got, want) {
		t.Errorf("幅 30 の列 = %v, want %v", got, want)
	}

	first := Columns(token.RunnerColumns(), 72)
	if !reflect.DeepEqual(first, Columns(token.RunnerColumns(), 72)) {
		t.Errorf("同じ幅で結果が異なる: %v", columnIDs(first))
	}
	first[0].Width = 999
	if Columns(token.RunnerColumns(), 72)[0].Width == 999 {
		t.Error("返り値への書き換えが次の呼び出しに影響している")
	}
}

// 間隔は列と列の間だけに入る（最終列の後ろには入らない）。
func TestColumnsFitCountsGutterBetweenColumnsOnly(t *testing.T) {
	cols := []token.Column{
		{ID: "A", Title: "A", Width: 5, Right: false},
		{ID: "B", Title: "B", Width: 5, Right: false},
	}
	want := columnPrefix + 5 + columnGutter + 5
	if !columnsFit(cols, want) || columnsFit(cols, want-1) {
		t.Errorf("必要幅は %d のはずだが判定が合わない", want)
	}
	// 列が無ければ常に収まる（負の間隔を作らない）。
	if !columnsFit(nil, 0) {
		t.Error("列が無いのに収まらないと判定された")
	}
}
