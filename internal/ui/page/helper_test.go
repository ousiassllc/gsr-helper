package page

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/appconfig"
	"github.com/ousiassllc/gsr-helper/internal/exec"
	"github.com/ousiassllc/gsr-helper/internal/runner"
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

// testRunnerDir は共有状態に載せる runner の同一性。中身が配られたことを見るための値。
const testRunnerDir = "/opt/runners/build01-1"

// state は本体の領域だけを指定した共有状態を返す。
//
// 領域を配る唯一の経路は SetState である（Overlay は SetSize を持たない。
// 持たせても次の共有状態で黙って巻き戻る）。
//
// **配色とキー定義以外（Result / Caps / Exec）も埋める。** 空にすると、共有状態の
// 一部だけを配る実装（Issue #32 以前の StateMsg{Keys, Styles}）でも検証が通る。
func state(w, h int) StateMsg {
	return StateMsg{
		Result: runner.Result{Runners: []runner.Runner{{Dir: testRunnerDir}}},
		Caps:   appconfig.Caps{Systemd: true, SudoUser: "ousiass"},
		Deps:   Deps{Exec: exec.NewFake()},
		Keys:   testKeys(),
		Styles: testStyles(),
		Dark:   true,
		BodyW:  w,
		BodyH:  h,
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
	// state は最後に受け取った共有状態。**中身を保つ**のは、件数だけを数えると
	// 一部のフィールドしか配らない実装でも検証が通るためである（Issue #32）。
	state StateMsg
	sizes int // 受け取った SizeMsg の回数（変化時のみ配られることを見る）
	size  SizeMsg
	tab   int  // AttachMsg で受け取ったタブ番号
	back  bool // esc を自分で解釈するか（Modal.HandlesBack が返す値）
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
		body: body, keys: nil, msgs: nil, states: 0, state: StateMsg{}, sizes: 0,
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
		m.state = msg
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

// wantFullState は配られた共有状態が欠けていないことを見る。w / h は配った本体の領域。
//
// **中身をフィールドごとに見る。** 配られた件数だけを数えると、配るのが
// StateMsg{Keys, Styles} だけに退行しても検証が通り、受け取ったモーダルが
// Result / Caps / Exec を持てないという Issue #32 の症状を見逃す。
func wantFullState(t *testing.T, stub *stubModal, w, h int) {
	t.Helper()

	got := stub.state
	if n := len(got.Result.Runners); n != 1 || got.Result.Runners[0].Dir != testRunnerDir {
		t.Errorf("配られた検出結果 = %+v, want %q 1 台", got.Result, testRunnerDir)
	}
	if !got.Caps.Systemd || got.Caps.SudoUser == "" {
		t.Errorf("配られた能力 = %+v, want 渡した値", got.Caps)
	}
	if got.Deps.Exec == nil {
		t.Error("配られた共有状態に Executor が無い（ドメイン層を呼ぶ道が渡っていない）")
	}
	if got.Keys.Global.Help.Help().Key == "" {
		t.Error("配られた共有状態にキー定義が無い")
	}
	if got.BodyW != w || got.BodyH != h {
		t.Errorf("配られた本体領域 = %dx%d, want %dx%d", got.BodyW, got.BodyH, w, h)
	}
	if stub.size.W == 0 || stub.size.H == 0 || stub.size.W >= w {
		t.Errorf("配られた領域 = %+v, want 枠の分を引いた値", stub.size)
	}
	if stub.tab != testTab {
		t.Errorf("配られたタブ番号 = %d, want %d", stub.tab, testTab)
	}
}
