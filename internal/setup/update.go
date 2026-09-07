package setup

import (
	"strings"

	"github.com/ousiassllc/gsr-helper/internal/runner"
	"github.com/ousiassllc/gsr-helper/internal/setup/tarball"
)

// UpdateSpec はバージョン更新の入力（FR-20〜FR-22）。
type UpdateSpec struct {
	// Runners は更新する runner。
	Runners []runner.Runner
	// Version は差し替える先のバージョン（表示用）。
	Version string
}

// PlanUpdate はバージョン一括更新の計画を立てる。
//
// 1 台につき「ドレイン停止 → 展開 → 起動」の順で組み立てる（FR-22）。
// 展開では `.runner` / `.credentials` / `.env` / `.path` / `_work` / `_diag` を
// 上書きしない（FR-21）。起動まで進めるのは、更新前に起動していた台だけである。
//
// systemd 管理外で稼働中の runner は対象から外す。停止させる手段が無いまま
// 本体のファイルを差し替えると、動作中の runner を壊すことになる。
func PlanUpdate(spec UpdateSpec) (Plan, error) {
	if len(spec.Runners) == 0 {
		return Plan{}, ErrNoTargets
	}

	units := make([]Unit, 0, len(spec.Runners))
	skipped := make([]string, 0)
	for _, r := range spec.Runners {
		if !updatable(r) {
			skipped = append(skipped, r.Name())
			continue
		}
		units = append(units, updateUnit(r))
	}
	if len(units) == 0 {
		return Plan{}, ErrNoTargets
	}

	return Plan{
		Kind:         KindUpdate,
		Units:        units,
		NeedsToken:   false,
		NeedsTarball: true,
		Version:      spec.Version,
		Warnings:     updateWarnings(units, skipped),
		Notes:        updateNotes(spec.Version),
	}, nil
}

// updatable は更新の対象にしてよいかを返す。
//
// systemd 管理下なら停止と復帰ができる。管理外でも停止していれば差し替えは安全に
// 行える。危険なのは「停止できないのに動いている」組み合わせだけである。
func updatable(r runner.Runner) bool {
	if r.Managed == runner.ManagedSystemd && r.UnitName != "" {
		return true
	}
	return !r.Running()
}

// updateUnit は 1 台分の更新手順を組み立てる。
func updateUnit(r runner.Runner) Unit {
	running := r.Running()
	steps := make([]Step, 0, 3)

	if running && r.UnitName != "" {
		steps = append(steps, Step{
			Kind: StepDrain, Phase: "ドレイン停止", Name: "", Args: nil,
			Dir: r.Dir, Action: ActionUpdate, TokenIndex: NoToken, Keep: nil, Env: nil,
		})
	}

	steps = append(steps, Step{
		Kind: StepExtract, Phase: "展開", Name: "", Args: nil,
		Dir: r.Dir, Action: ActionUpdate, TokenIndex: NoToken, Keep: tarball.PreservedNames(),
		Env: nil,
	})

	if running && r.UnitName != "" {
		steps = append(steps, Step{
			Kind: StepCommand, Phase: "起動", Name: "./svc.sh", Args: []string{"start"},
			Dir: r.Dir, Action: ActionUpdate, TokenIndex: NoToken, Keep: nil, Env: nil,
		})
	}

	return Unit{Name: r.Name(), Dir: r.Dir, Runner: r, WasRunning: running, Steps: steps}
}

// updateWarnings は確認ダイアログの「影響」に出す行を返す。
//
// 稼働中の台があることを必ず載せるのは、更新が一時停止を伴う操作だからである
// （FR-22）。確認は「対象と影響の提示」を必須段とする（functional.md の操作フロー）
// ため、影響が空のままだと dialog.Confirm の規則でブロックごと落ちてしまう。
func updateWarnings(units []Unit, skipped []string) []string {
	out := make([]string, 0, 4)

	if names := runningNames(units); len(names) > 0 {
		out = append(out,
			"⚠ 更新のため一時停止します（実行中ジョブの完了を待ちます）: "+strings.Join(names, ", "),
		)
	}
	if len(skipped) > 0 {
		out = append(out,
			"⚠ systemd 管理外で稼働中のため対象から外しました: "+strings.Join(skipped, ", "),
			"  先に停止してから再度実行してください",
		)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// runningNames は更新前に起動していた runner 名を返す。
func runningNames(units []Unit) []string {
	out := make([]string, 0, len(units))
	for _, u := range units {
		if u.WasRunning {
			out = append(out, u.Name)
		}
	}
	return out
}

// updateNotes は更新の補足を返す（FR-21 / FR-22）。
func updateNotes(version string) []string {
	notes := []string{
		".runner / .credentials / .env / .path / _work / _diag は保持されます。",
		"更新前に起動していた runner のみ、更新後に起動状態へ戻します。",
	}
	if version != "" {
		notes = append(notes, "runner バージョン: "+version+"（SHA-256 を検証してから展開します）")
	}
	return notes
}
