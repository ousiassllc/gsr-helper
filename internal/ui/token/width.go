package token

// 端末幅の基準。
const (
	// WidthTarget は表示を保証する最小の端末幅（非機能要件のユーザビリティ）。
	WidthTarget = 80
	// WidthMin はこれを下回ると表示不能とする幅。縮退の判断は template.Frame が行う。
	WidthMin = 60
)

// 列の識別子。ColumnRules との突き合わせと、
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
	ColTarget     = "TARGET"
	ColSize       = "SIZE"
	ColFiles      = "FILES"
	ColPath       = "PATH"
	ColLog        = "LOG"
	ColUpdated    = "UPDATED"
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
// ためである。この 3 列は RunnerColumnRules の Drop に含まれないので、幅が足りない場合は
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
// 同じ RunnerColumnRules に従って落ちる。必要幅は行頭 6 + 列幅合計 69 +
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

// DiskColumns は Disk タブの削除候補一覧の列を返す（screens.md の Disk タブ）。
//
// 必要幅は行頭 6（カーソル 1 + 間隔 1 + チェックボックス 3 + 間隔 1）+ 列幅合計 70 +
// 列間 3 = 79 セルで、WidthTarget（80）で 1 セル余る。行頭の内訳は molecule の
// columnPrefix と揃えること。
//
// 各列の幅の根拠は次のとおりである（いずれも lipgloss.Width での実測値）。
//   - TARGET 25: docker の対象名で最も長い「docker / ビルドキャッシュ」が 25 セルで、
//     ちょうど収まる。runner の対象名（「<runner 名> / _work/<リポジトリ>」）は
//     runner 名とリポジトリ名の長さで際限なく伸びるため幅を決める根拠にならず、
//     中略される前提で読む。docker の内訳は訳語が固定なので収めきれる。
//   - SIZE 11: 集計中の表示「（集計中…）」が 11 セルある（三点リーダ … は East Asian
//     Width が Ambiguous で 1 セル幅なので、6 文字でも 12 セルにはならない）。
//     数値（102.4M）や「✗ 失敗」（6 セル）より長く、この文字列が幅を決める。
//   - FILES 9: 7 桁（1,234,567）まで区切り付きで収まる。
//   - PATH 25: 一覧の最終列であり、organism/table が bubbles/table のセル余白の分だけ
//     最終列を 1 セル狭めるため実効 24 セルである。この列には選択できない理由が載り
//     （molecule/listrow.DiskTargetRow）、理由は 22 セル以内に収める約束なので
//     （disk.busyReason の doc）中略されない。パスは atom.Path が中間を中略して収める。
//
// SIZE と FILES を右寄せにするのは、桁の違う数値を縦に並べて比較する列だからである。
func DiskColumns() []Column {
	return []Column{
		{ID: ColTarget, Title: "TARGET", Width: 25, Right: false},
		{ID: ColSize, Title: "SIZE", Width: 11, Right: true},
		{ID: ColFiles, Title: "FILES", Width: 9, Right: true},
		{ID: ColPath, Title: "PATH", Width: 25, Right: false},
	}
}

// ColumnRules は幅が足りないときの列の落とし方。一覧の区画ごとに持つ。
//
// 落とし方を区画の定義と一緒に持つのは、**タブを 1 枚足すたびに共有の並びを
// 直さずに済むようにするため**である。screens.md が定める順は runner を並べる一覧の
// ものであり（RunnerColumnRules）、別の列を持つタブ（Disk / Logs / Doctor）は自分の
// 順を宣言する。持たせないと、落とす順に載っていない列は末尾から落ちるしかない。
//
// ゼロ値は「順の指定なし・落とさない列なし」であり、molecule.Columns は末尾から
// 落とす（列が 1 つ残るまで）。
type ColumnRules struct {
	// Drop は落とす順。先に挙げた列から落とし、収まった時点で止まる。
	Drop []string
	// Keep は幅が足りなくても落とさない列。Drop に含めても落とさない
	// （2 つの定義が食い違っても常時表示の保証が崩れないようにする）。
	Keep []string
}

// RunnerColumnRules は runner を並べる一覧（Runners / Jobs / 孤児ユニット）の
// 落とし方を返す（screens.md の「端末幅による列の省略」）。
func RunnerColumnRules() ColumnRules {
	return ColumnRules{
		Drop: []string{ColWork, ColVersion, ColManaged, ColScope},
		Keep: []string{ColName, ColSvc, ColJob},
	}
}

// DiskColumnRules は Disk タブの削除候補一覧の落とし方を返す。
//
// TARGET と SIZE は落とさない。**どれを消すとどれだけ空くのか**がこの画面の目的で
// あり（FR-28）、対象名か容量のどちらかが消えると選ぶ判断ができない。
//
// 先に落とすのは PATH である。対象名（TARGET）に runner 名と `_work` 配下の相対位置が
// 入っており、パスはその補足だからである。次が FILES で、削除の判断はサイズで足りる。
func DiskColumnRules() ColumnRules {
	return ColumnRules{
		Drop: []string{ColPath, ColFiles},
		Keep: []string{ColTarget, ColSize},
	}
}

// SizeColumnWidth はサイズ列（SIZE）の幅。
//
// atom.Bytes の最長表記（`1023.9K` の 7 桁）に合わせてある。定数にするのは、
// 表記を変えたときに列幅との食い違いをテストで検出できるようにするためである
// （atom.TestBytesFitsColumnWidth）。
const SizeColumnWidth = 7

// LogColumns は Logs タブのファイル一覧の列を返す。
//
// 必要幅は行頭 2（カーソル 1 + 間隔 1。選択できない一覧なのでチェックボックスの
// ガターは無い）+ 列幅合計 66 + 列間 3 = 71 セルで、WidthTarget（80）に収まる。
//
// RUNNER を持つのは、この一覧が runner をまたいで `_diag` のログを 1 つに並べる
// ためである（対象の runner を選ぶ画面を別に設けない）。
func LogColumns() []Column {
	return []Column{
		{ID: ColRunner, Title: "RUNNER", Width: 13, Right: false},
		{ID: ColLog, Title: "LOG", Width: 30, Right: false},
		{ID: ColSize, Title: "SIZE", Width: SizeColumnWidth, Right: true},
		{ID: ColUpdated, Title: "UPDATED", Width: 16, Right: false},
	}
}

// LogColumnRules は Logs タブのファイル一覧の落とし方を返す。
//
// 最初に落とすのは UPDATED である。並びが更新時刻の降順であること（FR-23）は
// 列が無くても順序から読み取れる。次が RUNNER で、選択中のログがどの runner の
// ものかは本文の見出しに出るため、狭い端末では一覧から省ける。
//
// LOG（ファイル名）と SIZE は落とさない。FR-23 が要求する「ログの一覧（サイズ付き）」
// そのものであり、落とすと一覧の目的を満たせない。
func LogColumnRules() ColumnRules {
	return ColumnRules{
		Drop: []string{ColUpdated, ColRunner},
		Keep: []string{ColLog, ColSize},
	}
}
