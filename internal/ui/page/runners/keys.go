package runners

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/runner"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/runnerdetail"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/runnerop"
)

// キー入力の解釈と、操作の対象の決め方を集める。1 ファイル 300 行の上限に
// 収めるための分割で、同じディレクトリなので合計行数は変わらない（Issue #35）。

// handleKey はキー入力を解釈する。
//
// 入力中とモーダル表示中は**親へ差し戻さない**。グローバルキーを閉じ込められるのは
// この判定を持つ page だけであり（page.GlobalKeyMsg の doc）、ここで差し戻すと
// 確認中に打った q でアプリが終わる。
func (m Model) handleKey(press tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	// 直前の打鍵に対する案内は次の打鍵で消す（page/disk の handleKey と同じ）。
	m.notice = ""

	var cmd tea.Cmd
	switch {
	case m.tbl.Filtering(), m.overlay.Active():
		return m.forward(press)
	case key.Matches(press, m.st.Keys.Global.Help):
		cmd = m.overlay.OpenHelp()
	case key.Matches(press, m.st.Keys.List.Enter):
		cmd = m.openDetail()
	case key.Matches(press, m.st.Keys.Runner.Logs):
		// ログは他のタブへ移る操作で、確認も実行も伴わない。サービス制御
		// （ops.HandleKey）より前に置くのは、ListOps がログを含まないためここで
		// 拾わないと既定の分岐へ落ちて親へ差し戻されるからである。
		cmd = m.openLogs()
	case key.Matches(press, m.st.Keys.Global.Back):
		m.back()
	case m.isSetupKey(press):
		cmd = m.openSetup(press)
	default:
		// runner の操作キー（s / x / X / d / R / E）はここで解釈する。判定は
		// key.Matches で行い、キー文字列のリテラルでは比較しない。**解釈したキーは
		// 一覧へも親へも渡さない**（渡すと 1 打鍵で操作と別の解釈が両方走る）。
		if c, ok := m.ops.HandleKey(press, runnerop.ListOps(), m.targets()); ok {
			cmd = c
			break
		}
		// 自分が解釈しないキーは一覧へ渡し、同時に親へ差し戻す。タブ切替・再読み込み・
		// 終了を解釈するのは親であり、一覧のキーと衝突しないことは keymap の
		// 重複検査（keymap.Set.Contexts の「一覧画面（通常モード）」）が担保する。
		// **この経路が二重解釈の起きる場所である。** 自前のキー集合を Set に足す
		// タブは、そのキーが同時に有効になるコンテキストを Contexts へ登録すること
		// （登録漏れは TestContextsCoverEverySetField が落とす）。
		next, c := m.forward(press)
		return next, tea.Batch(c, page.BubbleKey(press))
	}
	return m, tea.Batch(m.chrome(), cmd)
}

// targets は操作の対象を返す。一括選択があればそれ全部、無ければカーソル位置の
// 1 件である（FR-08。選び方そのものは runnerop.Targets が持つ）。
//
// 孤児ユニットの行は対象にしない。対応する runner ディレクトリが無く、
// サービス制御の対象として渡せる runner.Runner を持たないためである
// （区画そのものが選択不可なので checked には現れないが、カーソルは置ける）。
func (m Model) targets() []runner.Runner {
	checked := m.tbl.Checked()
	out := make([]runner.Runner, 0, len(checked))
	for _, r := range checked {
		if !r.isOrphan {
			out = append(out, r.runner)
		}
	}

	cur, ok := m.tbl.Selected()
	if cur.isOrphan {
		ok = false
	}
	return runnerop.Targets(out, cur.runner, ok)
}

// openDetail はカーソル位置の runner の詳細画面を開く。
//
// 孤児ユニットの行では開かない。孤児ユニットには対応する runner ディレクトリが
// 無く、詳細画面の項目（スコープ・バージョン・ディレクトリ）を埋められないためである。
func (m *Model) openDetail() tea.Cmd {
	cur, ok := m.tbl.Selected()
	if !ok || cur.isOrphan {
		return nil
	}
	return runnerdetail.Open(&m.overlay, cur.runner, m.st.Caps)
}

// back は esc の「選択のクリア / 1 つ前の状態へ戻る」を処理する。
//
// 選択を先に解くのは、選択したまま絞り込みを解除すると画面外の対象が選択されたまま
// 残るためである（screens.md のグローバルキー）。
func (m *Model) back() {
	// 直近の操作の結果も消す。esc は「1 つ前の状態へ戻る」であり、読み終えた
	// 報告を消す手段が他に無いと、次の操作まで状態行に残り続ける。
	m.ops.ClearStatus()
	if len(m.tbl.Checked()) > 0 {
		m.tbl.ClearSelection()
		return
	}
	m.tbl.ClearFilter()
}

// isSetupKey は Setup タブへ移す操作キー（n / D / u）かを返す。
func (m Model) isSetupKey(press tea.KeyPressMsg) bool {
	k := m.st.Keys.Runner
	return key.Matches(press, k.Add) || key.Matches(press, k.Delete) || key.Matches(press, k.Update)
}

// openSetup は追加・削除・バージョン更新を Setup タブへ引き渡す。
//
// **確認ダイアログはここで出さない。** 同じ操作の確認を移動元と移動先の 2
// 箇所で組み立てると、起点によって確認の中身が食い違いうる。実行前プレビューは
// 計画（setup.Plan）から組み立てる必要があり、その計画を持つのは Setup タブ側
// である（functional.md の操作フロー「起点によって確認の強さを変えない」）。
//
// 可否は移動の前に判定する。押しても何も起きないキーを作らないためであり、
// 判定はフッタに出す理由（action.Allow）と同じものを使う。
func (m *Model) openSetup(press tea.KeyPressMsg) tea.Cmd {
	k := m.st.Keys.Runner

	op := page.SetupAdd
	binding := k.Add
	targets := []runner.Runner(nil)
	switch {
	case key.Matches(press, k.Delete):
		op, binding, targets = page.SetupRemove, k.Delete, m.targets()
	case key.Matches(press, k.Update):
		op, binding, targets = page.SetupUpdate, k.Update, m.targets()
	}

	if reason, ok := m.setupBlocked(binding, targets); !ok {
		m.notice = reason
		return nil
	}
	return page.OpenTab(page.TabSetup, page.SetupRequestMsg{Op: op, Runners: targets})
}

// setupBlocked は Setup タブへ渡す操作の可否を判定し、不可なら理由を返す。
func (m Model) setupBlocked(b key.Binding, targets []runner.Runner) (string, bool) {
	k := page.BindingKey(b)
	if len(targets) == 0 {
		ok, why := m.actions.Allowed(k, runner.Runner{}, m.st.Caps)
		return why, ok
	}
	for _, r := range targets {
		if ok, why := m.actions.Allowed(k, r, m.st.Caps); !ok {
			return why, false
		}
	}
	return "", true
}
