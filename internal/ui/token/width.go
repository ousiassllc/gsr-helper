package token

// 端末幅の基準。
const (
	// WidthTarget は表示を保証する最小の端末幅（非機能要件のユーザビリティ）。
	WidthTarget = 80
	// WidthMin はこれを下回ると表示不能とする幅。縮退の判断は template.Frame が行う。
	WidthMin = 60
)

// 列の識別子。ColumnsAlways / ColumnDropOrder との突き合わせと、
// molecule の行の組み立てで参照する。見出しと別に持つのは、Jobs タブの
// "WORKER PID" や "_work" のように見出しの表記が識別子として使えないためである。
const (
	ColName       = "NAME"
	ColScope      = "SCOPE"
	ColManaged    = "MANAGED"
	ColSvc        = "SVC"
	ColJob        = "JOB"
	ColVersion    = "VERSION"
	ColWork       = "_WORK"
	ColUnit       = "UNIT"
	ColNote       = "NOTE"
	ColRunner     = "RUNNER"
	ColRepository = "REPOSITORY"
	ColElapsed    = "ELAPSED"
	ColWorkerPID  = "WORKER_PID"
)

// Column は一覧の列 1 つ分の定義。organism.Table が table.Column に変換する。
type Column struct {
	ID    string // 列の識別子
	Title string // 見出し
	Width int    // 表示幅（セル数）
	Right bool   // 右寄せにするか
}

// RunnerColumns は Runners タブの列を返す。
//
// 幅は WidthTarget（80）に全列が収まるように定めてある。必要幅は
// 行頭 6（カーソル 1 + 間隔 1 + チェックボックス 3 + 間隔 1）+ 列幅合計 67 +
// 列間 6 = 79 セルで、80 で 1 セル余る。行頭の内訳は molecule の columnPrefix と
// 揃えること。ここを広げると、保証する幅で列が落ちる。
func RunnerColumns() []Column {
	return []Column{
		{ID: ColName, Title: "NAME", Width: 16, Right: false},
		{ID: ColScope, Title: "SCOPE", Width: 10, Right: false},
		{ID: ColManaged, Title: "MANAGED", Width: 7, Right: false},
		{ID: ColSvc, Title: "SVC", Width: 10, Right: false},
		{ID: ColJob, Title: "JOB", Width: 9, Right: false},
		{ID: ColVersion, Title: "VERSION", Width: 9, Right: false},
		{ID: ColWork, Title: "_WORK", Width: 6, Right: true},
	}
}

// OrphanColumns は Runners タブ下部の孤児ユニット区画の列を返す。
//
// 必要幅は行頭 6 + 列幅合計 71 + 列間 2 = 79 セルで、WidthTarget（80）に全列が
// 収まる。UNIT の幅はユニット名（actions.runner.foo-bar.old01.service = 36 セル）が
// そのまま入るように取ってある。末尾を切り詰めると名前の識別に使う部分が消える
// ためである。この 3 列は ColumnDropOrder に含まれないので、幅が足りない場合は
// molecule.Columns が末尾（NOTE）から落とす。
//
// NOTE が注記「（対応ディレクトリなし）」の 24 セルより 1 セル広いのは、NOTE が
// この区画の最終列であり、organism が bubbles/table のセル余白の分だけ最終列を
// 1 セル狭めるためである。ちょうど 24 にすると幅 80 でも注記が必ず中略される。
func OrphanColumns() []Column {
	return []Column{
		{ID: ColUnit, Title: "UNIT", Width: 36, Right: false},
		{ID: ColSvc, Title: "SVC", Width: 10, Right: false},
		{ID: ColNote, Title: "NOTE", Width: 25, Right: false},
	}
}

// JobColumns は Jobs タブの列を返す。
//
// _work は ColWork を識別子に持つため、幅が足りない場合は Runners タブと
// 同じ ColumnDropOrder に従って落ちる。必要幅は行頭 6 + 列幅合計 69 +
// 列間 4 = 79 セルで、WidthTarget（80）に全列が収まる。_work は残余幅を
// 割り当てた結果であり、パスは atom.Path が中間を中略して収める。
func JobColumns() []Column {
	return []Column{
		{ID: ColRunner, Title: "RUNNER", Width: 13, Right: false},
		{ID: ColRepository, Title: "REPOSITORY", Width: 20, Right: false},
		{ID: ColElapsed, Title: "ELAPSED", Width: 8, Right: false},
		{ID: ColWorkerPID, Title: "WORKER PID", Width: 10, Right: true},
		{ID: ColWork, Title: "_work", Width: 18, Right: false},
	}
}

// ColumnsAlways は幅が足りなくても落とさない列の識別子を返す。
//
// molecule.Columns は ColumnDropOrder に加えてこの一覧も参照し、両者が食い違っても
// 常時表示が崩れないようにしている。
func ColumnsAlways() []string {
	return []string{ColName, ColSvc, ColJob}
}

// ColumnDropOrder は幅が足りない場合に列を落とす順を返す（screens.md の
// 「端末幅による列の省略」）。
func ColumnDropOrder() []string {
	return []string{ColWork, ColVersion, ColManaged, ColScope}
}
