package pagetest

import (
	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/ui/atom"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
)

// タブが page.Overlay へ足すモーダルを模した中身を置く。
//
// **登録・開封・共有状態のどれでも非 nil の Cmd を返す。** 呼び出し側が
// page.Overlay.Register / Open の戻り値を捨てていないことを、種類に依らず
// 確かめられるようにするためである（page.Overlay.Register の doc）。

// EchoKind は検証用モーダルの種類。本番のどの種類とも綴りが重ならないようにする。
const EchoKind page.ModalKind = "pagetest.echo"

// EchoMsg は Echo が受け取った Msg を Cmd の結果として返すための包み。
type EchoMsg struct{ Msg tea.Msg }

// Echo は受け取った Msg を記録し、同じ Msg を Cmd で返すモーダルの中身。
//
// ポインタで tea.Model を実装するのは、Update が返す値ではなく記録そのものを
// テストから見たいためである（Spy と同じ理由）。
type Echo struct {
	Got []tea.Msg // 受け取った Msg。届いた順に並ぶ
}

// tea.Model を実装していることをコンパイル時に確かめる。
var _ tea.Model = (*Echo)(nil)

// NewEcho は何も受け取っていない中身を返す。
func NewEcho() *Echo { return &Echo{Got: nil} }

// Init は何も発行しない。開くタイミングは page.Overlay が決める。
func (e *Echo) Init() tea.Cmd { return nil }

// Update は受け取った Msg を記録し、その Msg を包んだ Cmd を返す。
func (e *Echo) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	e.Got = append(e.Got, msg)
	echo := EchoMsg{Msg: msg}
	return e, func() tea.Msg { return echo }
}

// View は固定の文字列を返す。
func (e *Echo) View() tea.View { return tea.NewView("echo") }

// Received は指定した型の Msg を受け取った件数を返す。
//
// 型引数で問うのは、寿命の通知（page.ShutdownMsg など）がモーダルへ吸われて
// いないことを、Msg の中身に触れずに確かめるためである。
func Received[T tea.Msg](e *Echo) int {
	n := 0
	for _, m := range e.Got {
		if _, ok := m.(T); ok {
			n++
		}
	}
	return n
}

// EchoModal は Echo を包んだモーダルを返す。page.Overlay.Register に渡す。
func EchoModal(e *Echo) page.Modal {
	return page.Modal{
		Model: e,
		Title: func(tea.Model) string { return "echo" },
		Hints: func(tea.Model) []atom.Hint {
			return []atom.Hint{{Key: "y", Desc: "実行", Enabled: true, Reason: ""}}
		},
		HandlesBack: nil,
	}
}
