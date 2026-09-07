package setup

import (
	"errors"
	"strings"

	"github.com/ousiassllc/gsr-helper/internal/exec/mask"
	"github.com/ousiassllc/gsr-helper/internal/runner"
)

// ErrNoTargets は対象が 1 台も無い場合のエラー。
var ErrNoTargets = errors.New("対象の runner がありません")

// RemoveSpec は削除の入力（FR-17〜FR-19）。
type RemoveSpec struct {
	// Runners は削除する runner。
	Runners []runner.Runner
	// Root は gsr-helper 自身が root で動いているか（appconfig.Caps.Root）。
	// 真なら config.sh remove の手順へ RootEnv を渡す。
	Root bool
}

// PlanRemove は削除の計画を立てる。
//
// 発行するのは `svc.sh stop` → `svc.sh uninstall` → `config.sh remove --token`
// の順である（FR-17）。**runner ディレクトリは削除しない**（FR-18）。
func PlanRemove(spec RemoveSpec) (Plan, error) {
	if len(spec.Runners) == 0 {
		return Plan{}, ErrNoTargets
	}

	units := make([]Unit, 0, len(spec.Runners))
	busy := make([]string, 0)
	for _, r := range spec.Runners {
		if r.Busy() {
			busy = append(busy, r.Name())
		}
		units = append(units, removeUnit(r, spec.Root))
	}

	return Plan{
		Kind:         KindRemove,
		Units:        units,
		NeedsToken:   true,
		NeedsTarball: false,
		Version:      "",
		Warnings:     busyWarnings(busy),
		Notes:        removeNotes(units),
	}, nil
}

// removeUnit は 1 台分の削除手順を組み立てる。
//
// サービス化されていない runner には svc.sh の手順を入れない。存在しない
// ユニットに対する uninstall は必ず失敗し、そこで計画全体が中止されてしまう。
func removeUnit(r runner.Runner, root bool) Unit {
	steps := make([]Step, 0, 3)
	if r.UnitName != "" {
		steps = append(steps,
			Step{
				Kind: StepCommand, Phase: "サービス停止", Name: "./svc.sh", Args: []string{"stop"},
				Dir: r.Dir, Action: ActionRemove, TokenIndex: NoToken, Keep: nil, Env: nil,
			},
			Step{
				Kind: StepCommand, Phase: "サービス削除", Name: "./svc.sh", Args: []string{"uninstall"},
				Dir: r.Dir, Action: ActionRemove, TokenIndex: NoToken, Keep: nil, Env: nil,
			},
		)
	}

	args := []string{"remove", "--token", mask.Placeholder}
	steps = append(steps, Step{
		Kind: StepCommand, Phase: "登録解除", Name: "./config.sh", Args: args,
		Dir: r.Dir, Action: ActionRemove, TokenIndex: len(args) - 1, Keep: nil,
		Env: RootEnv(root),
	})

	return Unit{Name: r.Name(), Dir: r.Dir, Runner: r, WasRunning: r.Running(), Steps: steps}
}

// busyWarnings はジョブ実行中の runner を含む場合の警告を返す（FR-19）。
func busyWarnings(busy []string) []string {
	if len(busy) == 0 {
		return nil
	}
	return []string{
		"⚠ ジョブ実行中: " + strings.Join(busy, ", "),
		"  先に d（ドレイン停止）でジョブの完了を待つことを推奨します",
	}
}

// removeNotes は runner ディレクトリを残すことと、その場所を返す（FR-18）。
func removeNotes(units []Unit) []string {
	out := make([]string, 0, len(units)+1)
	out = append(out, "runner ディレクトリは削除されません:")
	for _, u := range units {
		out = append(out, "  "+u.Dir)
	}
	return out
}
