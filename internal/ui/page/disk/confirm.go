package disk

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/disk"
	"github.com/ousiassllc/gsr-helper/internal/ui/atom"
	"github.com/ousiassllc/gsr-helper/internal/ui/organism/dialog"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
)

// 確認ダイアログに出す文面（screens.md の Disk タブのドライラン画面）。
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

// クリーンアップの確認ダイアログを page.Overlay へ登録する包み。構造は
// page/runnerdetail/modal.go を写している（種類の定数・Open・New・Update の
// 振り分け・wrap）。**写す価値があるのは構造ではなく、そこに埋まっている決まりで
// ある。** 決定は page.Do で発行元のタブへ戻し、page.ResultMsg で包んで page 本体に
// 受けさせ、内側の部品が発行した Cmd は宛先を明示して自分へ帰す。この 3 つを外すと、
// 「y を押したのに何も起きない」という**エラーもログも残らない**壊れ方をする。
//
// 名前が runnerdetail と違うのは、この包みがタブと同じパッケージ（page/disk）に
// 同居するためである。Kind / Open / New をそのまま名乗ると、タブの New（Model の
// 構築関数）と衝突する。1 ファイル 300 行・1 ディレクトリ 2000 行の制約下で確認
// ダイアログのためだけにサブパッケージを 1 つ増やす価値は無いと判断した。

// confirmKind はクリーンアップの確認ダイアログの種類。
//
// 綴りが他の種類と重ならないことが Overlay の前提である（重複登録は panic する。
// page.Overlay.Register の doc）。タブ名を接頭辞に付けて、後続 Issue が持ち込む
// 別の確認（サービス停止・runner 削除）と衝突しないようにしてある。
const confirmKind page.ModalKind = "diskconfirm"

// confirmOpenMsg は確認ダイアログを開く指示。出す文面を丸ごと渡す。
//
// 開く指示を Msg にするのは、Overlay がモーダルの種類ごとの引数を知らずに済むように
// するためである（page.Modal の doc）。
type confirmOpenMsg struct {
	Input dialog.ConfirmInput
}

// openConfirm は確認ダイアログを開く。
//
// 種類と Msg の組を呼び出し側に書かせないために用意する（取り違えると開かない、
// あるいは別のモーダルへ Msg が届く）。**戻り値の Cmd は呼び出し側まで返すこと**
// （page.Overlay.Open の doc）。
func openConfirm(o *page.Overlay, in dialog.ConfirmInput) tea.Cmd {
	return o.Open(confirmKind, confirmOpenMsg{Input: in})
}

// confirmModal は確認ダイアログのモーダル。dialog.Confirm を tea.Model として包む。
//
// dialog.Confirm 自身は bubbles 流の署名（具体型を返す Update と View() string）を
// 保つ。tea.Model にすると呼び出し側で型アサーションが要り、ダイアログを直接
// 組み立てて検証する経路も回りくどくなるためである。
type confirmModal struct {
	// tab は自分が乗っているタブ番号。page.AttachMsg で Overlay から受け取る。
	// 決定を page へ返す Cmd を包むために持つ（page.AttachMsg の doc）。
	tab int
	dlg dialog.Confirm
}

// tea.Model を実装していることをコンパイル時に確かめる。
var _ tea.Model = confirmModal{}

// newConfirm は確認ダイアログのモーダルを組み立てる。page.Overlay.Register に渡す。
func newConfirm(st page.StateMsg) page.Modal {
	return page.Modal{
		Model: confirmModal{tab: 0, dlg: dialog.NewConfirm(st.Keys, st.Styles)},
		Title: confirmTitle,
		Hints: confirmHints,
		// esc は常に 1 枚閉じる＝キャンセルである。確認ダイアログには入力も編集も
		// 無く、esc に「取消してから閉じる」という 2 段階の意味が無い。閉じるだけで
		// 削除が走らないことは、実行の起点が dialog.DecidedMsg{Confirmed: true} の
		// 1 本しか無いこと（Model.onResult）で担保される。
		HandlesBack: nil,
	}
}

// Init は何も発行しない。開くタイミングは Overlay が決める。
func (m confirmModal) Init() tea.Cmd { return nil }

// Update は自分のタブ番号・決定・開く指示・共有状態・大きさ・キーを振り分ける。
func (m confirmModal) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case page.AttachMsg:
		m.tab = msg.Tab
		return m, nil
	case dialog.DecidedMsg:
		// ダイアログが返した決定を page へ差し戻す。ドメイン層を呼べるのは page
		// 階層だけ（atomic-design.md の依存の規則）であり、モーダルはここで削除を
		// 実行できない。**page.Do で包む**ことでタブを切り替えても発行元の page へ
		// 戻り、page.ResultMsg で包むことで Overlay が自分自身へ配り直さない。
		res := page.ResultMsg{Kind: confirmKind, Msg: msg}
		return m, page.Do(m.tab, func() tea.Msg { return res })
	case confirmOpenMsg:
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
// ある。ヘルプを重ねた後やタブを切り替えた後に届くと宛先を失い、**y を押したのに
// 何も起きない**という形で黙って消える。
//
// **包む相手はダイアログが自分で発行した Cmd に限る。** bubbletea / bubbles が
// 解釈する Msg（終了・順次実行）を包むとランタイムへ届かなくなる（page.Do の doc）。
// dialog.Confirm はカーソルもタイマーも持たないため、ランタイム宛の Msg を発行しない。
func (m confirmModal) wrap(cmd tea.Cmd) tea.Cmd {
	if cmd == nil {
		return nil
	}
	return page.Do(m.tab, func() tea.Msg {
		return page.ModalMsg{Kind: confirmKind, Msg: cmd()}
	})
}

// View は対象・影響・コマンド・補足・問いを返す。見出しは枠が描く。
func (m confirmModal) View() tea.View { return tea.NewView(m.dlg.View()) }

// confirmTitle はモーダルの見出しを返す。
func confirmTitle(model tea.Model) string {
	m, ok := model.(confirmModal)
	if !ok {
		return ""
	}
	return m.dlg.Title()
}

// confirmHints は確認ダイアログのフッタに出すキーヒントを返す。
func confirmHints(model tea.Model) []atom.Hint {
	m, ok := model.(confirmModal)
	if !ok {
		return nil
	}
	return m.dlg.Hints()
}

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
