package page

import (
	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/ui/atom"
)

// ModalKind はモーダルの種類。page/<tab> が自分の種類を定数で宣言する。
//
// 文字列にするのは、種類の集合を 1 つの iota の並びに集めないためである。集めると
// タブを 1 つ足すたびにこの共有ファイルへ定数を足すことになる（Confirm・
// DiffApproval・DrainWaiter・ProgressList はそれぞれ別の Issue が持ち込む）。
type ModalKind string

// ModalHelp は ? の全キー一覧。どの画面にもあるため Overlay が自分で登録する。
//
// これ以外の種類は画面が Register で足す（runner の詳細画面は
// page/runnerdetail、確認や差分承認は各 Issue が持ち込む）。
const ModalHelp ModalKind = "help"

// SizeMsg はモーダル 1 枚が使える領域。Overlay が枠と見出しの分を引いてから配る。
//
// StateMsg と分けているのは、StateMsg の BodyW / BodyH が本体の領域（枠の外側）で
// あり、モーダルの中身が使える領域とは別の値だからである。
type SizeMsg struct {
	W int
	H int
}

// AttachMsg はモーダルに、自分が乗っているタブ番号を知らせる。Register で届く。
//
// モーダルが発行した Cmd の結果は、包まなければ「そのとき選択中のタブ」へ配られる
// （TabMsg の doc）。モーダルは自分がどのタブに乗っているかを知る手段を持たないため、
// 登録した時点で Overlay が配る。**受け取った側はドメイン層の呼び出しと page へ返す
// 決定を Do(Tab, ...) で包むこと。** 包まないと、タブを切り替えている間に返ってきた
// 結果が別のタブへ渡って静かに失われる。
//
// タブ番号を Modal の構築関数の引数にしないのは、モーダルを足す Issue ごとに
// 「tab を受け取って持ち回る」規律を書き写させないためである。
type AttachMsg struct {
	Tab int
}

// ModalMsg は宛先の種類を明示したモーダル宛の Msg。
//
// Overlay.Update はキー以外の Msg も最上位の 1 枚にしか渡さないため、背後のモーダルが
// 始めた処理の結果は最上位に食われ、閉じた後に届いた結果は誰にも届かない。包んで
// 宛先を明示すると、**開いていなくても、最上位でなくても**その種類へ届く。開いた
// 瞬間に処理を始めるモーダル（ログの購読、差分の計算）はこれで結果を回収する。
type ModalMsg struct {
	Kind ModalKind
	Msg  tea.Msg
}

// ResultMsg はモーダルが、自分を開いた page へ返す決定。
//
// **Overlay ではなく page が解釈する。** page の Update はモーダルへの転送
// （Overlay.Handles を見る default）より前にこの case を置くこと。転送してしまうと
// 決定は発行元のモーダル自身へ戻り、そこで捨てられる。Overlay.Handles も
// ResultMsg には偽を返し、取り違えを構造で塞いでいる。
//
// 発行側は AttachMsg で受け取ったタブ番号を使って Do(tab, ...) で包む。タブを
// 切り替えても発行元の page へ戻り、モーダルを閉じた後でも page が受けるため
// 決定が宛先を失わない。
type ResultMsg struct {
	Kind ModalKind // どのモーダルの決定か
	Msg  tea.Msg   // 決定の中身（organism.ChosenMsg など）
}

// Modal は重ねられるモーダル 1 枚の定義。
//
// 中身は tea.Model として持つ。独自の interface を作らない規約（interface は
// Executor / doctor.Check / tea.Model の 3 つに限る。components/overview.md）に収めつつ、
// 「キーを Update で受け、View で描く」という約束を bubbletea と同じ形にできる。
// 開く・大きさが変わる・共有状態が届く・自分のタブ番号を知る、はすべて Msg として
// Model へ渡るため、Overlay は個々のモーダルの API を知らない。
//
// 見出しとキーヒントは tea.Model から取れないため、Model を引数に取る関数として
// 登録する（Model は Update で差し替わるので、値を閉じ込めると古くなる）。
type Modal struct {
	Model tea.Model                     // 中身。キー・SizeMsg・StateMsg・AttachMsg はここへ渡る
	Title func(m tea.Model) string      // モーダルの見出し
	Hints func(m tea.Model) []atom.Hint // フッタに出すキーヒント
	// HandlesBack は esc をモーダル自身が解釈するかを返す。nil なら常に 1 枚閉じる。
	//
	// 真を返す間、esc は最上位のモーダルへ渡り Overlay は閉じない。入力や編集の取消
	// （「この項目の編集をやめる」）を、閉じる操作より先に解釈させるためである。
	// 「今 esc を自分で使うか」だけを返させることで、Overlay は種類ごとの分岐を
	// 持たずに順序だけを与えられる（organism/table が確定後の解除と入力中の取消を
	// 書き分けているのと同じ切り分け）。
	HandlesBack func(m tea.Model) bool
}

// handlesBack は指定した種類のモーダルが esc を自分で解釈するかを返す。
//
// 送る前に問うのは、渡した後では「取消を処理し終えた」状態になっていて、解釈したか
// どうかを区別できないためである。
func (o Overlay) handlesBack(kind ModalKind) bool {
	m, ok := o.s.modals[kind]
	if !ok || m.HandlesBack == nil {
		return false
	}
	return m.HandlesBack(m.Model)
}
