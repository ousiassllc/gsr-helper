package setup

import (
	"github.com/ousiassllc/gsr-helper/internal/setup"
	"github.com/ousiassllc/gsr-helper/internal/ui/organism/dialog"
)

// 計画（setup.Plan）を確認ダイアログの中身へ落とす。**タブ固有の判断はここに残す**
// ——モーダルそのもの（page/setupmodal）は中身を組み立てない。

// confirmInput は計画から確認ダイアログの中身を組み立てる（FR-16）。
//
// **計画をそのまま写す。** コマンド全文は setup.Plan が持つものを出し、UI で
// 組み直さない。組み直すと承認した文面と実際に発行される内容が食い違う
// （docs/ui/screens.md の確認ダイアログ）。トークンの位置は計画の時点から
// *** なので、ここでマスクし直す必要も無い。
func confirmInput(p setup.Plan) dialog.ConfirmInput {
	return dialog.ConfirmInput{
		Title:   p.Kind.String() + "の確認",
		Targets: targetLines(p),
		Impact:  p.Warnings,
		Command: commandLines(p),
		Note:    p.Notes,
	}
}

// targetLines は対象の行を返す。
//
// 追加はまだ存在しない runner なので、作成するディレクトリを対象として出す
// （screens.md の実行前の確認）。削除・更新は runner 名とスコープを出す。
func targetLines(p setup.Plan) []string {
	out := make([]string, 0, len(p.Units))
	for _, u := range p.Units {
		if p.Kind == setup.KindAdd {
			out = append(out, u.Dir)
			continue
		}
		out = append(out, u.Name+"  "+u.Runner.Scope.String())
	}
	return out
}

// commandLines は実行するコマンド全文を実行順に全件返す。
func commandLines(p setup.Plan) []string {
	out := make([]string, 0, len(p.Units)*4)
	for _, u := range p.Units {
		out = append(out, u.CommandLines()...)
	}
	return out
}
