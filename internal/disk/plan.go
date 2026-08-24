package disk

import (
	"errors"
	"fmt"
	"slices"

	"github.com/ousiassllc/gsr-helper/internal/disk/pathguard"
)

// pruneCommand は docker の未使用リソースを削除するコマンド。
// 破壊的な docker コマンドはこれ 1 本だけに固定する
// （docs/api/external-interfaces.md「docker」）。
var pruneCommand = []string{"docker", "system", "prune", "-f"}

// PlanClean は選択された対象から削除計画（ドライラン）を組み立てる（FR-30）。
//
// ここで全対象の検証を先に済ませるのは、確認画面に出す内容と実際に消すものを
// 一致させるためである。1 件でも検証を通らなければ計画そのものを作らない。
// 一部だけ通った計画を返すと、利用者は画面に出ていない対象が残ったことに気付けない。
//
// 保護された対象（Target.Protected が空でない）はパスの検証より先に弾く。ジョブ
// 実行中の _work を消さないこと（FR-31）を表示層だけの約束にしないためであり、
// pathguard.Validate は「許可サブツリー内か」しか見ないのでこの判定を肩代わりできない。
func PlanClean(targets []Target) (CleanPlan, error) {
	if len(targets) == 0 {
		return CleanPlan{}, errors.New("対象が選択されていません")
	}

	plan := CleanPlan{Paths: nil, Docker: false, Bytes: 0, Commands: nil}
	for _, t := range targets {
		if t.Protected != "" {
			return CleanPlan{}, fmt.Errorf("%s は削除できません: %s", t.Label, t.Protected)
		}
		plan.Bytes += t.Bytes
		if t.Docker {
			// docker の対象が複数選ばれても発行するコマンドは 1 本。
			plan.Docker = true
			continue
		}
		if err := pathguard.Validate(t.Base, t.Path); err != nil {
			return CleanPlan{}, fmt.Errorf("%s は削除できません: %w", t.Label, err)
		}
		plan.Paths = append(plan.Paths, t)
	}
	if plan.Docker {
		// 写しを載せる。計画は確認画面へ渡って外から読まれるため、パッケージ変数を
		// そのまま指すと呼び出し側の書き換えが次回以降の計画にまで残る。
		plan.Commands = append(plan.Commands, slices.Clone(pruneCommand))
	}
	return plan, nil
}
