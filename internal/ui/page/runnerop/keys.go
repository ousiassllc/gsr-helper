package runnerop

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/runner"
	"github.com/ousiassllc/gsr-helper/internal/ui/keymap"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/action"
)

// 「どの操作を、どのキーで、どの対象に対して起動するか」の対応を集める。
// 操作そのものの実行は run.go、確認と待機は confirm.go / drain.go にある。

// Targets は操作の対象を返す。選択中の runner があればそれ全部、無ければカーソル
// 位置の 1 件（FR-08 の一括操作）。
//
// 一覧の行の型はタブごとに違う（Runners は孤児ユニットと同じ型、Jobs はジョブ 1 件）
// ため、行から runner を取り出すところまではタブが行い、ここは選び方だけを持つ。
// 選び方をタブに書き写すと、片方だけが一括選択を見ない状態になりうる。
func Targets(checked []runner.Runner, cur runner.Runner, ok bool) []runner.Runner {
	if len(checked) > 0 {
		return checked
	}
	if !ok {
		return nil
	}
	return []runner.Runner{cur}
}

// ListOps は Runners タブが一覧で直接受ける操作を返す（screens.md の Runners タブの操作）。
//
// n / D / u / e / l はこの版のサービス制御の範囲外なので含めない。含めると
// 「押せるが何も起きない」経路ができる。可否の判定（action.Allow）は
// page.ReasonUnsupported で塞いだままである。
//
// 関数で返すのは、書き換えられる共有状態（パッケージ変数）を作らないためである。
func ListOps() []action.ID {
	return []action.ID{action.Start, action.Stop, action.Kill, action.Drain, action.Restart, action.Enable}
}

// JobsOps は Jobs タブが直接受ける操作を返す（FR-47。d / X / R）。
//
// l（ログ）は Logs タブを持ち込む Issue の担当なので含めない。フッタには
// keymap.RunnerKeys.JobsFooter が出し続け、可否の判定が未対応で塞ぐ。
func JobsOps() []action.ID {
	return []action.ID{action.Drain, action.Kill, action.Restart}
}

// HandleKey はキーが runner の操作キーなら起動し、真を返す。
//
// **判定は key.Matches で行い、キー文字列のリテラルで比較しない。** keymap でキーを
// 差し替えたときに、判定がコンパイルエラーも無く別の操作へ移る（または消える）のを
// 防ぐためである（action.ID の doc と同じ理由）。
//
// 真を返したキーは**親へ差し戻してはならない**。差し戻すと 1 打鍵で操作の起動と
// グローバルキーの解釈が両方走る（page.GlobalKeyMsg の doc）。
func (m *Model) HandleKey(press tea.KeyPressMsg, ops []action.ID, targets []runner.Runner) (tea.Cmd, bool) {
	for _, id := range ops {
		b, ok := binding(id, m.st.Keys.Runner)
		if ok && key.Matches(press, b) {
			return m.Start(id, targets), true
		}
	}
	return nil, false
}

// binding は操作に対応するキー定義を返す。
//
// 対応表を持つのは、押されたキーから操作を引き直さずに済ませるためである。
// キー文字列で表を引くと、キーを差し替えたときに黙って別の操作へ移る（Issue #34）。
func binding(id action.ID, keys keymap.RunnerKeys) (key.Binding, bool) {
	switch id {
	case action.Start:
		return keys.Start, true
	case action.Stop:
		return keys.Stop, true
	case action.Kill:
		return keys.Kill, true
	case action.Drain:
		return keys.Drain, true
	case action.Restart:
		return keys.Restart, true
	case action.Enable:
		return keys.Enable, true
	default:
		return key.Binding{}, false
	}
}
