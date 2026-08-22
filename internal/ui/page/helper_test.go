package page

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/ui/atom"
	"github.com/ousiassllc/gsr-helper/internal/ui/keymap"
	"github.com/ousiassllc/gsr-helper/internal/ui/token"
)

// テストは内部テスト（package page）にしてある。詳細画面が組み立てた操作リストへ
// 直接キーを届けるといった、実装の内側の経路を検証するためである。

// press はキー入力の Msg を作る。文字キーは Text、特殊キーは Code で表す
// （bubbletea v2 の Key.String は Text があればそれを、無ければ keystroke を返す）。
func press(k string) tea.KeyPressMsg {
	special := map[string]rune{
		"space": tea.KeySpace, "enter": tea.KeyEnter, "esc": tea.KeyEscape, "tab": tea.KeyTab,
		"up": tea.KeyUp, "down": tea.KeyDown,
	}
	if code, ok := special[k]; ok {
		return tea.KeyPressMsg{Code: code}
	}
	if rest, ok := strings.CutPrefix(k, "ctrl+"); ok {
		return tea.KeyPressMsg{Code: rune(rest[0]), Mod: tea.ModCtrl}
	}
	return tea.KeyPressMsg{Text: k, Code: []rune(k)[0]}
}

// testStyles は色を使わないスタイル。期待文字列に ANSI 列が混ざらないようにする。
func testStyles() token.Styles {
	return token.NewStyles(true, false)
}

// testKeys はキー定義の集約を返す。
func testKeys() keymap.Set { return keymap.New() }

// testTab は検証で使うタブ番号。0 以外にして、配られる番号が既定値でないことを見る。
const testTab = 2

// state は本体の領域だけを指定した共有状態を返す。
//
// 領域を配る唯一の経路は SetState である（Overlay は SetSize を持たない。
// 持たせても次の共有状態で黙って巻き戻る）。
func state(w, h int) StateMsg {
	return StateMsg{
		Keys: testKeys(), Styles: testStyles(), Dark: true, BodyW: w, BodyH: h,
	}
}

// stubModal は登録したモーダルが受け取った Msg を記録するテスト用の中身。
//
// 重なりの規則を種類に依らず検証するために使う（具体的なモーダルを混ぜると、
// 検証しているのが規則なのか中身なのか分からなくなる）。
type stubModal struct {
	body   string
	keys   []string
	msgs   []tea.Msg // キー・共有状態・大きさ・タブ番号以外に届いた Msg
	states int
	sizes  int // 受け取った SizeMsg の回数（変化時のみ配られることを見る）
	size   SizeMsg
	tab    int  // AttachMsg で受け取ったタブ番号
	back   bool // esc を自分で解釈するか（Modal.HandlesBack が返す値）
}

// stubEcho は stubModal が受け取った Msg をそのまま返す Cmd の結果。
//
// Register / Open が返す Cmd が捨てられていないことを、種類に依らず確かめるために置く。
type stubEcho struct{ Msg tea.Msg }

// tea.Model を実装していることをコンパイル時に確かめる。
var _ tea.Model = (*stubModal)(nil)

// newStub は本文を持つモーダルを組み立てる。
func newStub(body string) Modal {
	m := &stubModal{
		body: body, keys: nil, msgs: nil, states: 0, sizes: 0,
		size: SizeMsg{W: 0, H: 0}, tab: -1, back: false,
	}
	return Modal{
		Model: m,
		Title: func(tea.Model) string { return body },
		Hints: func(tea.Model) []atom.Hint {
			return []atom.Hint{{Key: "y", Desc: "実行", Enabled: true, Reason: ""}}
		},
		HandlesBack: func(model tea.Model) bool {
			s, ok := model.(*stubModal)
			return ok && s.back
		},
	}
}

func (m *stubModal) Init() tea.Cmd { return nil }

func (m *stubModal) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		m.keys = append(m.keys, msg.String())
	case StateMsg:
		m.states++
	case SizeMsg:
		m.size = msg
		m.sizes++
	case AttachMsg:
		m.tab = msg.Tab
	default:
		m.msgs = append(m.msgs, msg)
	}
	// 受け取った Msg を Cmd にして返す。呼び出し側が Cmd を捨てていないことを
	// 種類に依らず確かめられるようにするためである。
	echo := stubEcho{Msg: msg}
	return m, func() tea.Msg { return echo }
}

func (m *stubModal) View() tea.View { return tea.NewView(m.body) }

// stubOf は登録した stubModal を取り出す。
func stubOf(t *testing.T, o Overlay, kind ModalKind) *stubModal {
	t.Helper()

	m, ok := o.Modal(kind)
	if !ok {
		t.Fatalf("%q が登録されていない", kind)
	}
	stub, ok := m.Model.(*stubModal)
	if !ok {
		t.Fatalf("%q の Model = %T, want *stubModal", kind, m.Model)
	}
	return stub
}
