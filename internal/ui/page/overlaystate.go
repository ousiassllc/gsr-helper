package page

import (
	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/ui/keymap"
	"github.com/ousiassllc/gsr-helper/internal/ui/template"
	"github.com/ousiassllc/gsr-helper/internal/ui/token"
)

// Overlay が写しのあいだで共有する実体と、その状態をモーダルへ配る経路を集める。

// overlayState は Overlay の写しが共有する実体。
type overlayState struct {
	keys   keymap.Set
	styles token.Styles

	stack  []ModalKind // 重なり順。末尾が最上位
	modals map[ModalKind]Modal

	// last は最後に配られた共有状態。登録と開封でリプレイする。
	//
	// 持たないと、遅延登録したモーダルは次の周期まで Result / Caps / Exec を
	// 持てない。閉じているモーダルへ毎周期配らずに済ませる（開いた時点で最新を
	// 受け取れる）ための土台でもある。
	last StateMsg
	// size は最後に配ったモーダルの領域。変化したときだけ配り直すために持つ。
	size   SizeMsg
	width  int
	height int
}

// SetState は共有状態のスナップショットを**開いているモーダルにだけ**配る。
//
// 開いている詳細は 3 秒ごとの再検出を反映する必要がある。**反映しないと、モーダルを
// 開いたまま状態が変わった runner に対して古い可否で操作を提示してしまう。**
//
// 閉じているモーダルへは配らない。配ると、誰も見ていないヘルプを毎周期組み直す
// （pane.NewHelp が全行を描く）無駄が積み上がる。古い状態で描かれる心配は Open が
// 開く直前にリプレイすることで断ってある。領域は変化したときだけ配る（変わって
// いない SizeMsg は下位に再計算を強いるだけである）。
func (o Overlay) SetState(st StateMsg) tea.Cmd {
	o.s.keys, o.s.styles = st.Keys, st.Styles
	o.s.last = st
	o.s.width, o.s.height = st.BodyW, st.BodyH

	size := o.innerSize()
	resized := size != o.s.size
	o.s.size = size

	cmds := make([]tea.Cmd, 0, len(o.s.stack)*2)
	for _, kind := range o.s.stack {
		cmds = append(cmds, o.send(kind, st))
		if resized {
			cmds = append(cmds, o.send(kind, size))
		}
	}
	return tea.Batch(cmds...)
}

// replay は最新の共有状態と領域を 1 種類のモーダルへ配る。
//
// 登録した直後と、閉じていたモーダルを開く直前に呼ぶ。これがあるので SetState は
// 開いているモーダルだけを相手にできる。
func (o Overlay) replay(kind ModalKind) tea.Cmd {
	return tea.Batch(o.send(kind, o.s.last), o.send(kind, o.s.size))
}

// send は種類を指定してモーダルへ Msg を渡す。
func (o Overlay) send(kind ModalKind, msg tea.Msg) tea.Cmd {
	m, ok := o.s.modals[kind]
	if !ok || msg == nil {
		return nil
	}

	var cmd tea.Cmd
	m.Model, cmd = m.Model.Update(msg)
	o.s.modals[kind] = m
	return cmd
}

// innerSize はモーダルの中身が使える領域を返す。
func (o Overlay) innerSize() SizeMsg {
	padW, padH := template.ModalPadding()
	return SizeMsg{W: max(o.s.width-padW, 1), H: max(o.s.height-padH, 1)}
}
