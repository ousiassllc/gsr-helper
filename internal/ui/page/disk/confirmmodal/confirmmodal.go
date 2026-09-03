// Package confirmmodal は Disk タブのクリーンアップ確認ダイアログを
// page.Modal として組み立てる薄い包みである。
//
// **Setup タブの確認・破棄ダイアログとは別物である。** 構造（dialog.Confirm を
// 包むだけ）は同じだが、Setup 側は 1 パッケージに確認と破棄の 2 種類を持ち、
// page/runnerdetail・page/runnerop のように複数タブが共用する部品でもない。
// Disk タブの外からは使わないにもかかわらずサブパッケージへ出したのは、
// internal/ui/page/disk ディレクトリの行数予算（1 ディレクトリ 2000 行 + 10%）に
// 収めるためである。文面の組み立て（confirmInput）はドメイン（disk.CleanPlan）を
// 知る必要があるため disk パッケージ側に残す。ここが持つのは page.Modal としての
// 組み立てと配線だけで、Disk タブ固有の判断は一切持たない。
package confirmmodal

import (
	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/ui/atom"
	"github.com/ousiassllc/gsr-helper/internal/ui/organism/dialog"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
)

// Kind はクリーンアップの確認ダイアログの種類。
const Kind page.ModalKind = "diskconfirm"

// OpenMsg は確認ダイアログを開く指示。出す文面を丸ごと渡す。
//
// 開く指示を Msg にするのは、Overlay がモーダルの種類ごとの引数を知らずに済むように
// するためである（page.Modal の doc）。
type OpenMsg struct{ Input dialog.ConfirmInput }

// Open は確認ダイアログを開く。
//
// 種類と Msg の組を呼び出し側に書かせないために用意する（取り違えると開かない、
// あるいは別のモーダルへ Msg が届く）。**戻り値の Cmd は呼び出し側まで返すこと**
// （page.Overlay.Open の doc）。
func Open(o *page.Overlay, input dialog.ConfirmInput) tea.Cmd {
	return o.Open(Kind, OpenMsg{Input: input})
}

// modal は確認ダイアログのモーダル。dialog.Confirm を tea.Model として包む。
//
// dialog.Confirm 自身は bubbles 流の署名（具体型を返す Update と View() string）を
// 保つ。tea.Model にすると呼び出し側で型アサーションが要り、ダイアログを直接
// 組み立てて検証する経路も回りくどくなるためである。
type modal struct {
	// tab は自分が乗っているタブ番号。page.AttachMsg で Overlay から受け取る。
	tab int
	dlg dialog.Confirm
}

// tea.Model を実装していることをコンパイル時に確かめる。
var _ tea.Model = modal{}

// New は確認ダイアログのモーダルを組み立てる。page.Overlay.Register に渡す。
func New(st page.StateMsg) page.Modal {
	return page.Modal{
		Model: modal{tab: 0, dlg: dialog.NewConfirm(st.Keys, st.Styles)},
		Title: title,
		Hints: hints,
		// esc は常に 1 枚閉じる＝キャンセルである。確認ダイアログには入力も編集も
		// 無く、esc に「取消してから閉じる」という 2 段階の意味が無い。閉じるだけで
		// 削除が走らないことは、実行の起点が dialog.DecidedMsg{Confirmed: true} の
		// 1 本しか無いこと（disk.Model.onResult）で担保される。
		HandlesBack: nil,
	}
}

// Init は何も発行しない。開くタイミングは Overlay が決める。
func (m modal) Init() tea.Cmd { return nil }

// Update は自分のタブ番号・決定・開く指示・共有状態・大きさ・キーを振り分ける。
func (m modal) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case page.AttachMsg:
		m.tab = msg.Tab
		return m, nil
	case dialog.DecidedMsg:
		// ダイアログが返した決定を page へ差し戻す。ドメイン層を呼べるのは page
		// 階層だけ（atomic-design.md の依存の規則）であり、モーダルはここで削除を
		// 実行できない。page.Do で包むことでタブを切り替えても発行元の page へ戻り、
		// page.ResultMsg で包むことで Overlay が自分自身へ配り直さない。
		res := page.ResultMsg{Kind: Kind, Msg: msg}
		return m, page.Do(m.tab, func() tea.Msg { return res })
	case OpenMsg:
		m.dlg.SetInput(msg.Input)
		return m, nil
	case page.StateMsg:
		// 配色とキー定義だけを差し替える。作り直すと確認の途中で文面が消える
		// （dialog.Confirm.Restyle の doc）。
		m.dlg.Restyle(msg.Keys, msg.Styles)
		return m, nil
	case page.SizeMsg:
		m.dlg.SetSize(msg.W, msg.H)
		return m, nil
	default:
		var cmd tea.Cmd
		m.dlg, cmd = m.dlg.Update(msg)
		return m, m.wrap(cmd)
	}
}

// wrap はダイアログが発行した Cmd の結果を、このダイアログへ戻るように包む。
//
// 包まないと結果は「そのとき選択中のタブの最上位のモーダル」へ配られる
// （page.AttachMsg / page.ModalMsg の doc）。dialog.Confirm が返す唯一の Cmd は
// y / n / esc / enter の決定（dialog.DecidedMsg）であり、上の case が受ける前提で
// ある。**包む相手はダイアログが自分で発行した Cmd に限る。**
func (m modal) wrap(cmd tea.Cmd) tea.Cmd {
	if cmd == nil {
		return nil
	}
	return page.Do(m.tab, func() tea.Msg {
		return page.ModalMsg{Kind: Kind, Msg: cmd()}
	})
}

// View は対象・影響・コマンド・補足・問いを返す。見出しは枠が描く。
func (m modal) View() tea.View { return tea.NewView(m.dlg.View()) }

// title はモーダルの見出しを返す。
func title(model tea.Model) string {
	m, ok := model.(modal)
	if !ok {
		return ""
	}
	return m.dlg.Title()
}

// hints は確認ダイアログのフッタに出すキーヒントを返す。
func hints(model tea.Model) []atom.Hint {
	m, ok := model.(modal)
	if !ok {
		return nil
	}
	return m.dlg.Hints()
}
