package runnerop

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/runner"
	"github.com/ousiassllc/gsr-helper/internal/svc"
	"github.com/ousiassllc/gsr-helper/internal/ui/atom"
	"github.com/ousiassllc/gsr-helper/internal/ui/organism/dialog"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/action"
	"github.com/ousiassllc/gsr-helper/internal/ui/token"
)

// ConfirmKind は確認ダイアログのモーダルの種類。
const ConfirmKind page.ModalKind = "confirm"

const (
	// confirmTitleSuffix は確認の見出しに付ける語（「停止の確認」）。
	confirmTitleSuffix = "の確認"
	// busyLabel はジョブ実行中の runner を挙げる行の見出し。
	busyLabel = " ジョブ実行中: "
	// busyAdvice はドレイン停止を促す行。行頭を字下げして上の行の続きだと示す。
	busyAdvice = "  先に d（ドレイン停止）でジョブの完了を待つことを推奨します"
)

// confirmInput は操作と対象から確認ダイアログの中身を組む。
//
// 4 ブロックの割り当ては screens.md の「確認ダイアログ」「実行前の確認」のモックと
// atomic-design.md の ConfirmInput に従う。
//
// **実行するコマンドは全文を全件出す。** screens.md の確認フローが要求する
// 「実行コマンド全文の表示」であり、対象が多くて収まらない場合は Confirm 側が
// 高さに合わせて畳む（dialog の fitHeight は y/N の行を最後まで残す）。文字列は
// svc.CommandLine から引き、**UI 側で組み立て直さない**。組み立て直すと、承認した
// 内容と実際に走るコマンドが食い違いうる（svc/commandline.go の doc）。
func confirmInput(def action.Def, targets []runner.Runner) dialog.ConfirmInput {
	op, hasOp := action.SvcOp(def.ID)

	names := make([]string, 0, len(targets))
	cmds := make([]string, 0, len(targets))
	for _, r := range targets {
		names = append(names, r.Name())
		if hasOp {
			cmds = append(cmds, svc.CommandLine(op, r)...)
		}
	}

	return dialog.ConfirmInput{
		Title:   def.Desc + confirmTitleSuffix,
		Targets: names,
		Impact:  impact(def, targets),
		Command: cmds,
		// 補足は置かない。対象・影響・コマンドで足りており、毎回同じ定型文を
		// 足すと本当に読ませたい影響の行が埋もれる（ConfirmInput の doc）。
		Note: nil,
	}
}

// impact は影響と警告の行を返す。
//
// 操作そのものの影響（action.Def.Impact）に加え、対象にジョブ実行中の runner が
// 含まれる場合は警告とドレイン停止の案内を足す（functional.md の確認フロー図の
// 「ジョブ実行中? → 警告・ドレインを促す」）。
//
// フッタの理由（action の「ジョブ実行中です。先に d で…」）と別に持つのは、
// あちらが「押せない理由」でこちらが「押せるが危険である旨」だからである。停止・
// 強制停止・再起動はジョブ実行中でも押せる（塞がれるのは削除だけ）ので、警告は
// 確認ダイアログでしか出せない。
func impact(def action.Def, targets []runner.Runner) []string {
	out := make([]string, 0, 3)
	if def.Impact != "" {
		out = append(out, def.Impact)
	}
	if busy := busyNames(targets); len(busy) > 0 {
		out = append(out, token.IconWarn+busyLabel+strings.Join(busy, " "), busyAdvice)
	}
	return out
}

// busyNames はジョブ実行中の runner の名前を返す。
func busyNames(targets []runner.Runner) []string {
	out := make([]string, 0, len(targets))
	for _, r := range targets {
		if r.Busy() {
			out = append(out, r.Name())
		}
	}
	return out
}

// confirmModal は確認ダイアログを page.Modal として包む。
//
// 包み方（AttachMsg でタブ番号を受け、決定を page.Do で包んで page.ResultMsg に
// して返し、内側の Cmd を page.ModalMsg で包む）は page/runnerdetail/modal.go と
// 同じである。Model 自身は bubbles 流の署名を保ち、tea.Model にするのはこの包みだけ。
type confirmModal struct {
	// tab は自分が乗っているタブ番号。page.AttachMsg で Overlay から受け取る。
	tab int
	dlg dialog.Confirm
}

// tea.Model を実装していることをコンパイル時に確かめる。
var _ tea.Model = confirmModal{}

// NewConfirmModal は確認ダイアログのモーダルを組み立てる。
func NewConfirmModal(st page.StateMsg) page.Modal {
	return page.Modal{
		Model: confirmModal{tab: 0, dlg: dialog.NewConfirm(st.Keys.Confirm, st.Styles)},
		Title: confirmTitle,
		Hints: confirmHints,
		// **esc はダイアログ自身がキャンセルとして解釈する。** 真を返さないと
		// Overlay が esc を「1 枚閉じる」で消費し、dialog.ConfirmedMsg{OK: false} が
		// page へ届かない。届かないと保留した操作（pending）が残り続け、次に
		// 別の操作を確認したときに古い対象へ実行しうる（dialog.Confirm.Update の doc）。
		HandlesBack: func(tea.Model) bool { return true },
	}
}

// Init は何も発行しない。開くタイミングは Overlay が決める。
func (m confirmModal) Init() tea.Cmd { return nil }

// Update は中身の差し替え・共有状態・大きさ・決定・キーを振り分ける。
func (m confirmModal) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case page.AttachMsg:
		m.tab = msg.Tab
		return m, nil
	case dialog.ConfirmedMsg:
		// 決定を page へ差し戻す。page.Do で包むことでタブを切り替えても発行元へ
		// 戻り、page.ResultMsg で包むことで Overlay が自分自身へ配り直さない。
		res := page.ResultMsg{Kind: ConfirmKind, Msg: msg}
		return m, page.Do(m.tab, func() tea.Msg { return res })
	case dialog.ConfirmInput:
		m.dlg.SetInput(msg)
		return m, nil
	case page.StateMsg:
		m.dlg.Restyle(msg.Keys.Confirm, msg.Styles)
		return m, nil
	case page.SizeMsg:
		m.dlg.SetSize(msg.W, msg.H)
		return m, nil
	default:
		var cmd tea.Cmd
		m.dlg, cmd = m.dlg.Update(msg)
		return m, wrap(m.tab, ConfirmKind, cmd)
	}
}

// View は確認の中身を返す。見出しは枠（template.Modal）が描く。
func (m confirmModal) View() tea.View { return tea.NewView(m.dlg.View()) }

// confirmTitle はモーダルの見出しを返す。
func confirmTitle(model tea.Model) string {
	m, ok := model.(confirmModal)
	if !ok {
		return ""
	}
	return m.dlg.Title()
}

// confirmHints はフッタに出すキーヒントを返す。
func confirmHints(model tea.Model) []atom.Hint {
	m, ok := model.(confirmModal)
	if !ok {
		return nil
	}
	return m.dlg.Hints()
}
