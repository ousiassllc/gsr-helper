package doctor

import (
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/doctor"
	"github.com/ousiassllc/gsr-helper/internal/ui/atom"
	"github.com/ousiassllc/gsr-helper/internal/ui/organism/pane"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
)

// Kind は詳細画面のモーダルの種類。
const Kind page.ModalKind = "doctordetail"

// 詳細画面の見出し（screens.md の Doctor タブの詳細）。
const (
	labelDetail = "検出内容:"
	labelImpact = "影響:"
	labelRemedy = "推奨する対処:"
)

// indent は各見出しの下の本文の字下げ。
const indent = "  "

// recheckDesc は個別再実行のキーの説明。
const recheckDesc = "この項目を再実行"

// OpenMsg は詳細画面を開く指示。
type OpenMsg struct {
	Result doctor.CheckResult
}

// RecheckMsg は詳細画面から返る「この項目を再実行する」という決定。
//
// 実行そのものは page が行う。ドメイン層を呼べるのは page 階層だけである
// （atomic-design.md の依存の規則）。
type RecheckMsg struct {
	ID string
}

// openDetail は詳細画面を開く。戻りの Cmd は呼び出し側まで返すこと。
func openDetail(o *page.Overlay, r doctor.CheckResult) tea.Cmd {
	return o.Open(Kind, OpenMsg{Result: r})
}

// modal は診断結果の詳細画面。
//
// **対処コマンドは表示するだけで実行しない。** 診断の責務を超えるためであり、
// sudoers の編集・パッケージの導入・usermod はいずれも本ツールから行わない
// （security.md の「実装しないこと」）。この画面に実行のキーを置かないことが
// その方針の実装である。
type modal struct {
	tab    int
	st     page.StateMsg
	result doctor.CheckResult
	pane   pane.Detail
}

// tea.Model を実装していることをコンパイル時に確かめる。
var _ tea.Model = modal{}

// newModal は詳細画面のモーダルを組み立てる。
func newModal(st page.StateMsg) page.Modal {
	return page.Modal{
		Model: modal{tab: 0, st: st, result: doctor.CheckResult{}, pane: pane.NewDetail()},
		Title: modalTitle,
		Hints: modalHints,
		// esc は常に 1 枚閉じる。この画面に入力も編集も無い。
		HandlesBack: nil,
	}
}

// Init は何も発行しない。開くタイミングは Overlay が決める。
func (m modal) Init() tea.Cmd { return nil }

// Update は開く指示・共有状態・大きさ・キーを振り分ける。
func (m modal) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case page.AttachMsg:
		m.tab = msg.Tab
		return m, nil
	case OpenMsg:
		m.result = msg.Result
		m.pane.SetContent(m.lines())
		m.pane.GotoTop()
		return m, nil
	case page.StateMsg:
		m.st = msg
		m.pane.SetContent(m.lines())
		return m, nil
	case page.SizeMsg:
		m.pane.SetSize(msg.W, msg.H)
		m.pane.SetContent(m.lines())
		return m, nil
	case tea.KeyPressMsg:
		return m.handleKey(msg)
	default:
		var cmd tea.Cmd
		m.pane, cmd = m.pane.Update(msg)
		return m, cmd
	}
}

// handleKey は再実行のキーを解釈し、残りは情報部へ渡す。
//
// 再実行に Global.Refresh（r）を使い専用の定義を足さないのは、一覧の r（全項目の
// 再実行）と同じ意味の操作だからである。別の Binding を重ねると、同時に有効な
// キーが 2 つになる（keymap.DiskKeys の doc と同じ理由）。
func (m modal) handleKey(press tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if key.Matches(press, m.st.Keys.Global.Refresh) && m.result.ID != "" {
		res := page.ResultMsg{Kind: Kind, Msg: RecheckMsg{ID: m.result.ID}}
		return m, page.Do(m.tab, func() tea.Msg { return res })
	}
	var cmd tea.Cmd
	m.pane, cmd = m.pane.Update(press)
	return m, cmd
}

// View は情報部を返す。
func (m modal) View() tea.View { return tea.NewView(m.pane.View()) }

// lines は詳細画面の本文を組み立てる。
//
// 出す節は 検出内容 / 影響 / 推奨する対処 の 3 つで、screens.md の Doctor タブの
// 詳細に対応する。**中身の無い節は出さない。** OK と SKIP には影響も対処も無く、
// 空の見出しだけが並ぶと「対処が要るのに書かれていない」と読める。
func (m modal) lines() []string {
	width := m.st.BodyW
	if width <= 0 {
		width = 60
	}

	var out []string
	add := func(label, body string) {
		if strings.TrimSpace(body) == "" {
			return
		}
		if len(out) > 0 {
			out = append(out, "")
		}
		out = append(out, m.st.Styles.Header.Render(label))
		for _, line := range wrapLines(body, width-len(indent)) {
			out = append(out, indent+line)
		}
	}

	add(labelDetail, m.result.Detail)
	add(labelImpact, m.result.Impact)
	add(labelRemedy, m.result.Remedy)
	return out
}

// modalTitle は見出しを返す（`✗ FAIL   時刻 / NTP 未同期   build01-2`）。
func modalTitle(model tea.Model) string {
	m, ok := model.(modal)
	if !ok || m.result.ID == "" {
		return ""
	}
	text, role := atom.DoctorStatus(stateToken(m.result.Status))
	parts := []string{
		m.st.Styles.Style(role).Render(text),
		m.result.Category + " / " + m.result.Summary,
	}
	if m.result.Target != "" {
		parts = append(parts, m.st.Styles.Muted.Render(m.result.Target))
	}
	return strings.Join(parts, "   ")
}

// modalHints はフッタに出すキーヒントを返す。
//
// esc は Overlay が足すため、ここでは再実行だけを出す。
func modalHints(model tea.Model) []atom.Hint {
	m, ok := model.(modal)
	if !ok {
		return nil
	}
	return []atom.Hint{{
		Key:     page.BindingKey(m.st.Keys.Global.Refresh),
		Desc:    recheckDesc,
		Enabled: m.result.ID != "",
		Reason:  "",
	}}
}

// wrapLines は本文を width で折り返す。
//
// 改行は保つ。対処（Remedy）は複数行のコマンド列であり、勝手に詰めると
// 貼り付けたときに動かないものになる。
func wrapLines(body string, width int) []string {
	if width < 1 {
		width = 1
	}
	var out []string
	for line := range strings.SplitSeq(body, "\n") {
		out = append(out, wrapOne(line, width)...)
	}
	return out
}

// wrapOne は 1 行を width で折り返す。
//
// 文字数ではなくルーン数で数える。日本語の文言をバイト数で切ると壊れた
// バイト列が画面に出る。表示幅で厳密に折るのは atom の責務だが、ここは
// 情報部（等幅の本文）なので概算で足りる。
func wrapOne(line string, width int) []string {
	r := []rune(line)
	if len(r) == 0 {
		return []string{""}
	}
	var out []string
	for len(r) > width {
		out = append(out, string(r[:width]))
		r = r[width:]
	}
	return append(out, string(r))
}
