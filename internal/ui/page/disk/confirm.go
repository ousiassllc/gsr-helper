package disk

import (
	"strings"

	"github.com/ousiassllc/gsr-helper/internal/disk"
	"github.com/ousiassllc/gsr-helper/internal/ui/atom"
	"github.com/ousiassllc/gsr-helper/internal/ui/organism/dialog"
)

// 確認ダイアログに出す文面（screens.md の Disk タブのドライラン画面）。
//
// モーダルとしての組み立て（page.Modal への配線）は
// internal/ui/page/disk/confirmmodal が持つ。ここに残るのは disk.CleanPlan を
// 文面へ落とす confirmInput だけである。ドメイン（disk パッケージ）を知る必要が
// あるため、ドメインを知らない汎用の包みとは分けてある。
const (
	// confirmHeading は確認ダイアログの見出し（screens.md の Disk タブ）。
	confirmHeading = "クリーンアップの確認"
	// impactPrefix は解放見込みの見出し。
	impactPrefix = "解放見込み: "
	// noteFiles はファイル削除が外部コマンドではないことの説明。
	//
	// 実行するコマンドの欄に載せないのは、載せると「そのコマンドを打てば同じことが
	// 起きる」と読めるためである。削除は internal/disk が自前で行い（進捗を出すため
	// と、シンボリックリンクを辿らないため）、rm も find も起動しない。
	noteFiles = "ファイル削除: 上記パスの再帰削除。シンボリックリンクは辿りません"
	// noteDockerScope は prune -f が消さないものの明示。出さないと、一覧に並ぶ内訳
	// （イメージ / ボリューム）まで消えると読める（disk.PruneReclaimable の doc）。
	noteDockerScope = "docker: イメージとボリュームは prune -f では削除されません"
	// noteIrreversible は取り消せないことの警告。
	noteIrreversible = "削除したファイルは復元できません。"
	// dockerTargetLabel は確認ダイアログに出す docker の対象名。
	//
	// 内訳（イメージ / ビルドキャッシュ）を並べない。実行するのは
	// docker system prune -f 1 本で、消えるのは選んだ内訳だけではないためである。
	dockerTargetLabel = "docker 未使用リソース"
)

// confirmInput は削除計画を確認ダイアログの文面に落とす。
//
// **文面の出どころは計画だけである。** 画面の選択状態から組み直すと、検証を通った
// 計画と利用者が見る文面が食い違いうる（PlanClean は docker の対象が複数選ばれても
// コマンドを 1 本にまとめる）。
func confirmInput(plan disk.CleanPlan) dialog.ConfirmInput {
	targets := make([]string, 0, len(plan.Paths)+1)
	for _, t := range plan.Paths {
		targets = append(targets, t.Path+"  "+atom.Bytes(t.Bytes)+"  "+atom.Files(t.Files)+" ファイル")
	}
	if plan.Docker {
		targets = append(targets, dockerTargetLabel)
	}

	commands := make([]string, 0, len(plan.Commands))
	for _, c := range plan.Commands {
		commands = append(commands, strings.Join(c, " "))
	}

	note := make([]string, 0, 3)
	if len(plan.Paths) > 0 {
		note = append(note, noteFiles)
	}
	if plan.Docker {
		note = append(note, noteDockerScope)
	}
	note = append(note, noteIrreversible)

	return dialog.ConfirmInput{
		Title:   confirmHeading,
		Targets: targets,
		Impact:  []string{impactPrefix + atom.Bytes(plan.Bytes)},
		Command: commands,
		Note:    note,
	}
}
