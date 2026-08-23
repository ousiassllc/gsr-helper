package disk

import (
	"path/filepath"
	"strings"

	"github.com/ousiassllc/gsr-helper/internal/disk"
	"github.com/ousiassllc/gsr-helper/internal/runner"
)

// 削除してよい対象かの判定を集める。
//
// clean.go（実行の一本道）から分けているのは、**保護の判定が削除経路とは別の理由で
// 変わる**ためである。ジョブ実行中のガード（FR-31）は security.md の
// 「ジョブ実行中の操作をガードする」に、対象の絞り込みは FR-30 に紐づく。
// 1 ファイル 300 行の上限に対して clean.go を薄く保つ狙いも兼ねる。

// cleanTargets は選択された行を削除計画の入力に変換する。
//
// disk.Usage をそのまま渡さず disk.Target に落とすのは、「集計しただけの行」が
// 削除計画に紛れ込まないようにするためである（disk.Target の doc）。
func cleanTargets(rows []row) []disk.Target {
	out := make([]disk.Target, 0, len(rows))
	for _, r := range rows {
		u := r.usage
		// 選べない理由はドメインまで運ぶ。表（organism/table）が選択を阻むだけに
		// すると、ジョブ実行中の保護（FR-31）が表示層だけの約束になる
		// （disk.Target.Protected の doc）。
		protected := u.Reason
		if u.Removable {
			protected = ""
		}
		out = append(out, disk.Target{
			Label:     u.Label,
			Base:      u.Base,
			Path:      u.Path,
			Bytes:     u.Bytes,
			Files:     u.Files,
			Docker:    u.Kind == disk.KindDocker,
			Protected: protected,
		})
	}
	return out
}

// reprotected は計画を立ててから承認するまでの間に保護へ転じた対象の表示名を返す。
// 保護へ転じた対象が無ければ空文字を返す。
//
// **Target.Protected だけでは足りない。** あれは disk.Scan がジョブの有無を見た
// 「その時点」の値であり、確認ダイアログには時間制限が無い。承認を待つ間にジョブが
// 始まった runner は Protected が空のままなので、PlanClean と Apply の双方の判定を
// 素通りし、実行中ジョブの _work が root 権限で消える（FR-31 が守るはずのもの）。
// ValidatePath は削除の直前に再検証されるのに busy 判定だけ据え置きでは、同じ種類の
// 穴が 1 つ残る。共有状態は 3 秒ごとに更新されるので、承認の直前に引き直せる。
//
// **_diag は対象にしない。** security.md の「ジョブ実行中の操作をガードする」が
// ブロックするのは _work 配下だけであり、_diag はジョブ実行中でも削除してよい。
func reprotected(plan disk.CleanPlan, runners []runner.Runner) string {
	for _, t := range plan.Paths {
		for _, r := range runners {
			if !r.Busy() || r.Dir != t.Base {
				continue
			}
			work := filepath.Join(r.Dir, workDirName)
			if t.Path == work || strings.HasPrefix(t.Path, work+string(filepath.Separator)) {
				return t.Label
			}
		}
	}
	return ""
}
