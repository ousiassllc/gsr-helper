package logs

import (
	"errors"
	"regexp"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	dlogs "github.com/ousiassllc/gsr-helper/internal/logs"
	"github.com/ousiassllc/gsr-helper/internal/ui/atom"
	"github.com/ousiassllc/gsr-helper/internal/ui/keymap"
	"github.com/ousiassllc/gsr-helper/internal/ui/molecule"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
	"github.com/ousiassllc/gsr-helper/internal/ui/token"
)

// 画面の組み立て（領域の配分・見出し・本文の絞り込みと強調）と、親へ返す
// ChromeMsg の中身を集める。

const (
	// inputFilter は入力中であることを状態行に出すときの名称（screens.md の入力中）。
	inputFilter = "フィルタ"
	// emptyMessage はログが 1 つも見つからないときの表示。
	emptyMessage = "ログがありません（runner の _diag に Runner_*.log / Worker_*.log が無い）"
	// noTargetMessage は本文に開いているものが無いときの表示。
	noTargetMessage = "一覧から選んで enter で開きます"

	// journalctl へ切り替えられない理由。screens.md の「無効な操作の表示」に倣い、
	// キーを消さずに理由を添える。
	reasonNoJournal = "journalctl が見つかりません"
	reasonNoUnit    = "systemd ユニットがありません"
	reasonNoTarget  = "開いているログがありません"

	// followOn / followOff は見出しに出す追従の状態（screens.md の Logs タブ）。
	followOn  = "追従: ON"
	followOff = "追従: OFF"

	// paneList / paneBody は状態行に出す操作中のペイン。
	paneList = "一覧"
	paneBody = "本文"

	// headerHeight は見出しの行数。
	headerHeight = 1
	// minListHeight / maxListHeight はファイル一覧に配る高さの下限と上限。
	//
	// 上限を置くのは、ログが 100 件あっても本文が潰れないようにするためである。
	// 下限は見出し 1 行 + 1 件で、一覧がまったく見えない状態を作らない。
	minListHeight = 2
	maxListHeight = 8
)

// errNoWorkerLog は `l` の宛先が無いことを表すエラーを返す。
func errNoWorkerLog(name string) error {
	return errors.New(name + " に Worker のログがありません")
}

// logsHelp は Logs タブが ? に出すキーのグループを返す。
func logsHelp(s keymap.Set) [][]key.Binding { return s.LogsHelp() }

// resize は本体の領域を一覧・見出し・本文へ配る。
//
// 一覧の高さは行数に合わせて縮める。ログが数件しか無いときに空行で本文を狭めない
// ためである。上限と下限で挟むのは maxListHeight の doc を参照。
func (m *Model) resize() {
	w, h := m.st.BodyW, m.st.BodyH
	list := min(max(len(m.tbl.Shown(sectionLogs))+1, minListHeight), maxListHeight)
	if body := h - list - headerHeight; body < minListHeight {
		// 本文に配れる高さが尽きるときは一覧を削る。本文が Logs タブの主役である。
		list = max(h-headerHeight-minListHeight, 0)
	}
	m.tbl.SetSize(w, list)
	m.body.SetSize(w, max(h-list-headerHeight, 0))
}

// applyLines は保持している行を絞り込み・強調して本文へ渡す。
//
// **絞り込みは素の行に対して行う。** 装飾済みの文字列に正規表現を当てると ANSI 列が
// 一致に混ざる（pane.Log が突き合わせを持たない理由でもある）。
func (m *Model) applyLines() {
	re, err := compileFilter(m.body.Filter())
	m.filterErr = err

	out := make([]string, 0, len(m.lines))
	for _, l := range m.lines {
		if re != nil && !re.MatchString(l.Text) {
			continue
		}
		out = append(out, m.styleLine(l))
	}
	m.body.SetContent(out)
}

// compileFilter はフィルタを正規表現に解く。空なら nil を返す（絞り込みなし）。
func compileFilter(s string) (*regexp.Regexp, error) {
	if s == "" {
		return nil, nil
	}
	return regexp.Compile(s)
}

// styleLine は重大度に応じて行を装飾する（FR-25 の強調表示）。
//
// 描くのは molecule.LogLine に任せ、ここは重大度から表示上の役割への対応づけだけを
// 持つ。ドメインの型（dlogs.Level）を molecule へ渡さないためである
// （atomic-design.md の依存の規則）。
func (m Model) styleLine(l dlogs.Line) string {
	return molecule.LogLine(l.Text, lineRole(l.Level), m.st.Styles)
}

// lineRole は重大度に対応する表示上の役割を返す。
func lineRole(l dlogs.Level) token.RoleToken {
	switch l {
	case dlogs.LevelError:
		return token.RoleFail
	case dlogs.LevelWarn:
		return token.RoleWarn
	case dlogs.LevelPlain:
		return token.RolePlain
	default:
		return token.RolePlain
	}
}

// render は一覧・見出し・本文を縦に並べる。
func (m Model) render() string {
	list := m.tbl.View()
	if list == "" {
		list = m.st.Styles.Muted.Render(emptyMessage)
	}
	body := m.body.View()
	if m.target.empty() {
		body = m.st.Styles.Muted.Render(noTargetMessage)
	}
	return strings.Join([]string{list, m.header(), body}, "\n")
}

// header は本文の見出しを返す（screens.md の Logs タブの 1 行目）。
//
// 対象の runner とログ名・追従・フィルタを 1 行にまとめる。幅に収まらない分は
// atom.Join が末尾から落とす。
func (m Model) header() string {
	if m.target.empty() {
		return m.st.Styles.Muted.Render(strings.Repeat("─", max(m.st.BodyW, 0)))
	}

	parts := []string{
		m.st.Styles.Accent.Render(m.target.runner.Name()),
		m.target.name(),
		m.followText(),
	}
	if f := m.body.FilterView(); f != "" {
		parts = append(parts, f)
	}
	return atom.Join(parts, "   ", m.st.BodyW, "")
}

// followText は追従の状態を返す。
func (m Model) followText() string {
	if m.body.Following() {
		return m.st.Styles.OK.Render(followOn)
	}
	return m.st.Styles.Muted.Render(followOff)
}

// chrome は親へ本体以外の状態を知らせる Cmd を返す。
func (m Model) chrome() tea.Cmd {
	c := page.ChromeMsg{
		Tab:    m.tab,
		Modal:  m.overlay.Active(),
		Input:  m.input(),
		Status: m.status(),
		Footer: m.footer(),
	}
	return func() tea.Msg { return c }
}

// input は入力中の名称を返す。入力中でなければ空文字を返す。
func (m Model) input() string {
	if m.body.Filtering() {
		return inputFilter
	}
	return ""
}

// status は状態行に出す page 側の文を返す。
//
// 優先するのは入力中・失敗・操作中のペインの順である。入力中はグローバルキーが
// 効かない状態なので必ず出し（screens.md の入力中）、次に利用者が手を打てる情報
// （追従の失敗・不正な正規表現）を出す。
func (m Model) status() string {
	switch {
	case m.input() != "":
		return "入力中: " + m.input()
	case m.filterErr != nil:
		return "フィルタが正規表現として不正です: " + m.filterErr.Error()
	case m.err != nil:
		return m.err.Error()
	default:
		return "ペイン: " + m.paneName()
	}
}

// paneName は操作中のペインの名前を返す。
func (m Model) paneName() string {
	if m.focus == focusList {
		return paneList
	}
	return paneBody
}

// footer はフッタのキーヒントを返す。
//
// 出す操作と表記は keymap.LogKeys.Footer に従い、Logs タブ側では持たない（キーと
// 説明文の出どころを 1 つにするため。jobs.footer と同じ理由）。一覧を操作している
// 間だけ `enter` を先頭に足す。
func (m Model) footer() []atom.Hint {
	if m.overlay.Active() {
		return m.overlay.Hints()
	}

	keys := m.st.Keys.Log
	out := make([]atom.Hint, 0, len(keys.Footer())+1)
	if m.focus == focusList {
		out = append(out, atom.Hint{
			Key:     page.BindingKey(m.st.Keys.List.Enter),
			Desc:    "開く",
			Enabled: true,
			Reason:  "",
		})
	}
	reason, allowed := m.journalAllowed()
	for _, f := range keys.Footer() {
		k := page.BindingKey(f.Binding)
		hint := atom.Hint{Key: k, Desc: f.Desc, Enabled: true, Reason: ""}
		if f.Binding.Help().Key == keys.Journal.Help().Key && !allowed && !m.target.journal {
			hint.Enabled, hint.Reason = false, reason
		}
		out = append(out, hint)
	}
	return out
}
