package jobs

import (
	"strconv"
	"strings"

	"github.com/ousiassllc/gsr-helper/internal/runner"
	"github.com/ousiassllc/gsr-helper/internal/ui/keymap"
	"github.com/ousiassllc/gsr-helper/internal/ui/molecule"
	"github.com/ousiassllc/gsr-helper/internal/ui/organism/table"
	"github.com/ousiassllc/gsr-helper/internal/ui/token"
)

// sectionJobs は実行中ジョブの区画の添字。Jobs タブは 1 区画だけを持つ。
const sectionJobs = 0

// row は Jobs タブの 1 行。ジョブ 1 件と、それを実行している runner の組。
//
// runner を持つのは、操作対象がジョブではなく runner だからである（FR-47）。
type row struct {
	runner runner.Runner
	worker runner.Process
}

// newTable は Jobs タブの一覧を組み立てる。
func newTable(keys keymap.Set, s token.Styles) table.Model[row] {
	return table.New(keys.List, s, table.SectionInput[row]{
		Title:   "",
		Columns: token.JobColumns(),
		Render:  renderJob,
		ID:      func(r row) string { return strconv.Itoa(r.worker.PID) },
		Match:   matchJob,
		// 選択できないのは、Jobs タブに一括操作が無いためである（操作対象は
		// カーソル位置のジョブを実行している runner 1 台）。
		Disabled:   nil,
		Selectable: false,
	})
}

// renderJob はジョブの行をセル列に変換する。
func renderJob(in table.RowInput[row]) []string {
	return molecule.JobRow(jobView(in.Item), in.Cols, in.Styles)
}

// matchJob は絞り込みの一致判定。runner 名を対象にする。
func matchJob(r row, q string) bool {
	return strings.Contains(strings.ToLower(r.runner.Name()), strings.ToLower(q))
}

// jobView はジョブを表示用の構造体に落とす。
//
// Repository を空にするのは、Runner.Worker（/proc 由来）にジョブのリポジトリ情報が
// 無いためである。値はログのパッケージ（Worker ログの解析）ができてから埋める。
// molecule 側は空の値を "-" として描く。
func jobView(r row) molecule.JobView {
	return molecule.JobView{
		Runner:     r.runner.Name(),
		Repository: "",
		Elapsed:    r.worker.Elapsed(),
		WorkerPID:  r.worker.PID,
		Work:       r.runner.WorkDir,
	}
}

// jobRows は検出結果を runner 横断のジョブ一覧に平坦化する。
//
// 並びは runner の順（検出結果はスコープ→名前で並んでいる）で、同じ runner の中は
// Worker の PID 昇順（attach が整えている）。再検出のたびに行が入れ替わらないよう、
// 検出結果の順序をそのまま使う。
func jobRows(runners []runner.Runner) []row {
	var out []row
	for _, r := range runners {
		for _, w := range r.Workers {
			out = append(out, row{runner: r, worker: w})
		}
	}
	return out
}
