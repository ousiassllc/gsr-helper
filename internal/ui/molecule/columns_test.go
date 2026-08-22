package molecule

import (
	"reflect"
	"slices"
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
		if got, want := columnIDs(Columns(all, token.WidthTarget, token.RunnerColumnRules())), columnIDs(all); !reflect.DeepEqual(got, want) {
			t.Errorf("%s: 幅 %d の列 = %v, want %v", name, token.WidthTarget, got, want)
		}
		for width := 120; width >= token.WidthMin; width-- {
			if cols := Columns(all, width, token.RunnerColumnRules()); !columnsFit(cols, width) {
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
		if got := columnIDs(Columns(token.RunnerColumns(), c.width, token.RunnerColumnRules())); !reflect.DeepEqual(got, c.want) {
			t.Errorf("幅 %d の列 = %v, want %v", c.width, got, c.want)
		}
	}
}

// 幅を 1 刻みで狭めても、落ちる順が飛ばず逆転しない。
//
// 落とす順（screens.md の「端末幅による列の省略」）は受け入れ条件そのものなので、
// 境界の 1 セルずれを個別の期待値ではなく走査で確かめる。
func TestColumnsDropOneByOneAsWidthShrinks(t *testing.T) {
	order := token.RunnerColumnRules().Drop
	prev := columnIDs(Columns(token.RunnerColumns(), 120, token.RunnerColumnRules()))
	dropped := 0

	for width := 119; width >= token.WidthMin; width-- {
		got := columnIDs(Columns(token.RunnerColumns(), width, token.RunnerColumnRules()))
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
		got := columnIDs(Columns(token.RunnerColumns(), width, token.RunnerColumnRules()))
		for _, id := range token.RunnerColumnRules().Keep {
			if !containsID(got, id) {
				t.Errorf("幅 %d で常に表示する列 %s が落ちた（%v）", width, id, got)
			}
		}
	}
	if got, want := columnIDs(Columns(token.RunnerColumns(), 30, token.RunnerColumnRules())), token.RunnerColumnRules().Keep; !reflect.DeepEqual(got, want) {
		t.Errorf("幅 30 の列 = %v, want %v", got, want)
	}

	first := Columns(token.RunnerColumns(), 72, token.RunnerColumnRules())
	if !reflect.DeepEqual(first, Columns(token.RunnerColumns(), 72, token.RunnerColumnRules())) {
		t.Errorf("同じ幅で結果が異なる: %v", columnIDs(first))
	}
	first[0].Width = 999
	if Columns(token.RunnerColumns(), 72, token.RunnerColumnRules())[0].Width == 999 {
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

// 入力に列があれば、どんな幅でも 1 列は返す。
//
// Jobs タブの列は 1 つも ColumnsAlways に含まれないため、末尾から落とす経路が
// 歯止め無く走ると全列が消える。列が 0 個になると見出しも行も描けず、幅 19 未満の
// Jobs タブが空表になる。表示不能とするかの判断は template.Frame の責務であり、
// molecule.Columns は最小の列を返して幅を超えることを許す。
func TestColumnsNeverReturnsEmptyForNonEmptyInput(t *testing.T) {
	for name, all := range columnSets() {
		for width := 20; width >= -10; width-- {
			if got := Columns(all, width, token.RunnerColumnRules()); len(got) == 0 {
				t.Errorf("%s: 幅 %d で列が 0 個になった", name, width)
			}
		}
	}

	// 落とし切ったあとに残るのは先頭の列（最も識別に使う列）である。
	for width := 10; width <= 18; width++ {
		if got, want := columnIDs(Columns(token.JobColumns(), width, token.RunnerColumnRules())), []string{token.ColRunner}; !reflect.DeepEqual(got, want) {
			t.Errorf("JobColumns: 幅 %d の列 = %v, want %v", width, got, want)
		}
	}
}

// 落とす順は区画ごとに宣言できる。同じ列でも順が違えば残る列が変わる。
//
// 共有の 1 本に固定すると、列の並びが違うタブ（Disk / Logs / Doctor）を足すたびに
// その並びを直すことになる（token.ColumnRules の doc）。
func TestColumnsFollowsPerSectionRules(t *testing.T) {
	all := []token.Column{
		{ID: token.ColName, Title: "NAME", Width: 10},
		{ID: token.ColScope, Title: "SCOPE", Width: 10},
		{ID: token.ColVersion, Title: "VERSION", Width: 10},
	}
	// 行頭 6 + 列幅 20 + 列間 1 = 27。1 列だけ落ちる幅を選ぶ。
	const width = 27

	dropScope := Columns(all, width, token.ColumnRules{
		Drop: []string{token.ColScope},
		Keep: []string{token.ColName},
	})
	dropVersion := Columns(all, width, token.ColumnRules{
		Drop: []string{token.ColVersion},
		Keep: []string{token.ColName},
	})

	if got := columnIDs(dropScope); !slices.Equal(got, []string{token.ColName, token.ColVersion}) {
		t.Errorf("SCOPE を先に落とす順の結果 = %v", got)
	}
	if got := columnIDs(dropVersion); !slices.Equal(got, []string{token.ColName, token.ColScope}) {
		t.Errorf("VERSION を先に落とす順の結果 = %v", got)
	}

	// 順を宣言しない区画は末尾から落とす（ゼロ値の意味）。
	if got := columnIDs(Columns(all, width, token.ColumnRules{})); !slices.Equal(
		got, []string{token.ColName, token.ColScope}) {
		t.Errorf("順を宣言しない区画の結果 = %v, want 末尾から落とす", got)
	}
}

// Keep が空の ColumnRules でも、all が空でなければ列が 1 つ以上残る。
//
// 落とす順（rules.Drop）を辿るループには末尾落としループにある「1 列残す」歯止めが
// 無く、Keep を宣言しない区画では最後の 1 列まで落ちて空表になっていた。仕様書が
// 3 箇所で約束する契約（「all が空でなければ結果も空にならない」）の回帰である。
// 自分の順を宣言するタブ（Disk / Logs / Doctor）はこの経路を通る。
func TestColumnsKeepsLastColumnWhenRulesHaveNoKeep(t *testing.T) {
	// Drop が全列を挙げ、Keep が空。幅はどの列も収まらない狭さにする。
	all := []token.Column{
		{ID: token.ColWork, Title: "_WORK", Width: 6, Right: true},
	}
	rules := token.ColumnRules{Drop: []string{token.ColWork}, Keep: nil}

	got := Columns(all, 10, rules)
	if len(got) == 0 {
		t.Fatalf("列が 0 個になった（Drop=%v Keep=%v）", rules.Drop, rules.Keep)
	}
	if want := []string{token.ColWork}; !slices.Equal(columnIDs(got), want) {
		t.Errorf("列 = %v, want %v", columnIDs(got), want)
	}
	// 最後の 1 列は幅が足りなくても残る。結果は width を超え得る（screens.md）。
	if columnsFit(got, 10) {
		t.Error("この幅では収まらないはずで、超過を許して残す契約が確かめられていない")
	}

	// 複数列でも、Drop を使い切るまでに 1 列で止まる。
	multi := []token.Column{
		{ID: token.ColName, Title: "NAME", Width: 16, Right: false},
		{ID: token.ColScope, Title: "SCOPE", Width: 10, Right: false},
		{ID: token.ColVersion, Title: "VERSION", Width: 9, Right: false},
	}
	multiRules := token.ColumnRules{
		Drop: []string{token.ColVersion, token.ColScope, token.ColName},
		Keep: nil,
	}
	for width := 20; width >= -10; width-- {
		cols := Columns(multi, width, multiRules)
		if len(cols) == 0 {
			t.Fatalf("幅 %d で列が 0 個になった", width)
		}
	}
	// 落とし切ったあとに残るのは Drop の最後に挙げた列（最も落としたくない列）。
	if got, want := columnIDs(Columns(multi, 1, multiRules)), []string{token.ColName}; !slices.Equal(got, want) {
		t.Errorf("幅 1 の列 = %v, want %v", got, want)
	}
}
