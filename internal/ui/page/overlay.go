package page

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/ui/atom"
	"github.com/ousiassllc/gsr-helper/internal/ui/keymap"
	"github.com/ousiassllc/gsr-helper/internal/ui/template"
	"github.com/ousiassllc/gsr-helper/internal/ui/token"
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

// Modal は重ねられるモーダル 1 枚の定義。
//
// 中身は tea.Model として持つ。独自の interface を作らない規約（interface は
// Executor / doctor.Check / tea.Model の 3 つに限る。components/overview.md）に収めつつ、
// 「キーを Update で受け、View で描く」という約束を bubbletea と同じ形にできる。
// 開く・大きさが変わる・共有状態が届く、はすべて Msg として Model へ渡るため、
// Overlay は個々のモーダルの API を知らない。
//
// 見出しとキーヒントは tea.Model から取れないため、Model を引数に取る関数として
// 登録する（Model は Update で差し替わるので、値を閉じ込めると古くなる）。
type Modal struct {
	Model tea.Model                     // 中身。キー・SizeMsg・StateMsg はここへ渡る
	Title func(m tea.Model) string      // モーダルの見出し
	Hints func(m tea.Model) []atom.Hint // フッタに出すキーヒント
}

// Overlay はモーダルの重なりを管理する。
//
// キーは最上位の 1 枚にのみ渡し、背後の page には届けない。確認中に打った x が
// 背後の一覧で別の停止操作として解釈されることを構造的に防ぐためである
// （screens.md のモーダル表示中）。esc で 1 枚だけ閉じるのもここで担保する。
//
// **種類ごとの分岐を持たない。** 重なりの規則（キーは最上位だけ・esc は 1 枚）を
// 種類の数と無関係にするためである。モーダルを増やす Issue は Register で自分の
// 種類を足すだけで、この共有ファイルを触らない。
//
// **コピーは重なりの実体を共有する。** stack のスライスと modals の map は写しても
// 同じ実体を指すため、page は直前の Update が返した 1 つの値だけを持つこと
// （organism/table.Model と同じ約束）。
type Overlay struct {
	keys   keymap.Set
	styles token.Styles

	stack  []ModalKind // 重なり順。末尾が最上位
	modals map[ModalKind]Modal

	width  int
	height int
}

// NewOverlay はモーダルの重なりを組み立てる。
//
// ヘルプ（ModalHelp）だけを登録した状態で返す。ヘルプに出すキーの範囲は一覧 +
// runner 操作（keymap.Set.RunnerListHelp）を既定とし、別の範囲を持つタブは
// SetHelpScope で差し替える。それ以外のモーダルは画面が Register で足す。
func NewOverlay(keys keymap.Set, s token.Styles, dark bool) Overlay {
	o := Overlay{
		keys:   keys,
		styles: s,
		stack:  nil,
		modals: make(map[ModalKind]Modal),
		width:  0,
		height: 0,
	}
	// 登録する部品には初期の共有状態を渡す。最初のリサイズと検出が届く前でも
	// 配色とキー定義を持った状態で描けるようにするためである（newTabs と同じ形）。
	st := StateMsg{Keys: keys, Styles: s, Dark: dark}
	o.Register(ModalHelp, newHelpModal(st, keymap.Set.RunnerListHelp))
	return o
}

// Register はモーダル 1 種類を登録する。同じ種類を登録し直すと差し替わる。
//
// 種類ごとに部品を 1 つだけ持つ（同じ種類を 2 枚重ねない）ため、重なりは種類の
// 並びだけで表せる。
func (o *Overlay) Register(kind ModalKind, m Modal) {
	o.modals[kind] = m
	o.send(kind, SizeMsg{W: o.innerWidth(), H: o.innerHeight()})
}

// SetHelpScope は ? に出すキーの範囲を差し替える。
//
// Set から範囲を選ぶ関数を渡すのは、配色やキー定義が差し替わったときに Overlay が
// 自分で組み直せるようにするためである（page が declare し直す必要が無い）。
func (o *Overlay) SetHelpScope(scope HelpScope) {
	o.Register(ModalHelp, newHelpModal(o.state(), scope))
}

// Open は種類を指定してモーダルを開く。既に開いていれば最上位へ動かす。
//
// 開くときに渡した Msg は中身の Model へそのまま届く。「何を開くか」（対象の runner や
// 確認の文面）を Msg で渡すことで、Overlay は種類ごとの引数を知らずに済む。
func (o *Overlay) Open(kind ModalKind, msg tea.Msg) tea.Cmd {
	if _, ok := o.modals[kind]; !ok {
		return nil
	}

	cmd := o.send(kind, msg)
	o.push(kind)
	return cmd
}

// OpenHelp は全キー一覧を開く。
func (o *Overlay) OpenHelp() {
	o.Open(ModalHelp, nil)
}

// Close は最上位のモーダルを 1 枚だけ閉じる。
//
// 1 枚ずつ閉じるのは、詳細画面からヘルプを開いた後の esc で詳細まで消えると、
// 利用者が「どこへ戻ったか」を追えなくなるためである。
func (o *Overlay) Close() {
	if len(o.stack) == 0 {
		return
	}
	o.stack = o.stack[:len(o.stack)-1]
}

// Modal は登録済みのモーダルを返す。
//
// 中身を具体型へ戻すのは登録した側の責任である（Overlay は tea.Model としてしか
// 持たない）。開いているかどうかは問わない。
func (o Overlay) Modal(kind ModalKind) (Modal, bool) {
	m, ok := o.modals[kind]
	return m, ok
}

// Active はモーダルを 1 枚以上開いているかを返す。ChromeMsg.Modal に載せる値である。
func (o Overlay) Active() bool { return len(o.stack) > 0 }

// SetState は共有状態のスナップショットを開いていないモーダルにも配る。
//
// 全部に配るのは、開いた瞬間に古い配色・古い検出結果で描かれることを防ぐためである
// （親 Model が全タブへ配るのと同じ理由）。開いている詳細も 3 秒ごとの再検出を反映する
// 必要がある。**反映しないと、モーダルを開いたまま状態が変わった runner に対して
// 古い可否で操作を提示してしまう。**
func (o *Overlay) SetState(st StateMsg) tea.Cmd {
	o.keys, o.styles = st.Keys, st.Styles
	o.width, o.height = st.BodyW, st.BodyH

	cmds := make([]tea.Cmd, 0, len(o.modals)*2)
	size := SizeMsg{W: o.innerWidth(), H: o.innerHeight()}
	for kind := range o.modals {
		cmds = append(cmds, o.send(kind, st), o.send(kind, size))
	}
	return tea.Batch(cmds...)
}

// SetSize はモーダルを置ける領域（本体の領域）を設定する。
func (o *Overlay) SetSize(w, h int) {
	o.width, o.height = w, h
	size := SizeMsg{W: o.innerWidth(), H: o.innerHeight()}
	for kind := range o.modals {
		o.send(kind, size)
	}
}

// Update はキーを最上位のモーダルにのみ渡す。
//
// esc はここで受けて 1 枚閉じる。最上位が受け取る前に閉じる判断をするのは、
// 「戻る」の意味をモーダルの種類ごとに実装させないためである。
func (o Overlay) Update(msg tea.Msg) (Overlay, tea.Cmd) {
	top, ok := o.top()
	if !ok {
		return o, nil
	}

	if press, isKey := msg.(tea.KeyPressMsg); isKey && key.Matches(press, o.keys.Global.Back) {
		o.Close()
		return o, nil
	}
	return o, o.send(top, msg)
}

// View は最上位のモーダルを枠に入れて返す。開いていなければ空文字を返す。
func (o Overlay) View() string {
	m, ok := o.topModal()
	if !ok {
		return ""
	}
	return template.Modal(template.ModalInput{
		Title:  o.styles.Header.Render(m.Title(m.Model)),
		Body:   m.Model.View().Content,
		Width:  o.width,
		Height: o.height,
	})
}

// Hints は最上位のモーダルのキーヒントを返す。開いていなければ nil を返す。
func (o Overlay) Hints() []atom.Hint {
	m, ok := o.topModal()
	if !ok {
		return nil
	}
	return m.Hints(m.Model)
}

// send は種類を指定してモーダルへ Msg を渡す。
//
// 受け取った Model を map へ書き戻す。map は写しても同じ実体を指すため、値レシーバの
// Update から呼んでも重なりの状態は 1 つに保たれる（Overlay の doc）。
func (o Overlay) send(kind ModalKind, msg tea.Msg) tea.Cmd {
	m, ok := o.modals[kind]
	if !ok || msg == nil {
		return nil
	}

	var cmd tea.Cmd
	m.Model, cmd = m.Model.Update(msg)
	o.modals[kind] = m
	return cmd
}

// state は登録済みのモーダルへ配る共有状態を組み立てる。
func (o Overlay) state() StateMsg {
	return StateMsg{Keys: o.keys, Styles: o.styles}
}

// top は最上位のモーダルの種類を返す。
func (o Overlay) top() (ModalKind, bool) {
	if len(o.stack) == 0 {
		return "", false
	}
	return o.stack[len(o.stack)-1], true
}

// topModal は最上位のモーダルを返す。
func (o Overlay) topModal() (Modal, bool) {
	kind, ok := o.top()
	if !ok {
		return Modal{}, false
	}
	m, ok := o.modals[kind]
	return m, ok
}

// innerWidth はモーダルの中身が使える幅を返す。
func (o Overlay) innerWidth() int {
	padW, _ := template.ModalPadding()
	return max(o.width-padW, 1)
}

// innerHeight はモーダルの中身が使える行数を返す。
func (o Overlay) innerHeight() int {
	_, padH := template.ModalPadding()
	return max(o.height-padH, 1)
}

// push はモーダルを重ねる。同じ種類が既にあるときは最上位へ動かす。
//
// 種類ごとに部品を 1 つしか持たないため、同じ種類を 2 枚積むと閉じても中身が
// 変わらない「閉じられないモーダル」に見える。
func (o *Overlay) push(kind ModalKind) {
	kept := make([]ModalKind, 0, len(o.stack)+1)
	for _, k := range o.stack {
		if k != kind {
			kept = append(kept, k)
		}
	}
	kept = append(kept, kind)
	o.stack = kept
}
