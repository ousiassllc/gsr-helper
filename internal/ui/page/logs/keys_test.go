package logs

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	dlogs "github.com/ousiassllc/gsr-helper/internal/logs"
	"github.com/ousiassllc/gsr-helper/internal/ui/atom"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/pagetest"
)

// キーの解釈（ペイン切替・追従・フィルタ・journalctl）を検証する。

// sample は行を直に持たせたタブを返す。
//
// 購読を張らずに本文の組み立てだけを見たい検証で使う。購読ごと辿る検証は
// stream_test.go にある。
func sample(t *testing.T, lines ...string) Model {
	t.Helper()

	st, r := withLogs(t)
	m := newTab(t, st)
	m.target = target{runner: r, file: dlogs.File{Name: "Worker_1.log"}, journal: false}
	for _, s := range lines {
		m.lines = append(m.lines, dlogs.NewLine(s))
	}
	m.applyLines()
	return m
}

// bubbled は Cmd に親への差し戻し（GlobalKeyMsg）が含まれるかを返す。
func bubbled(cmd tea.Cmd) bool {
	for _, c := range pagetest.Expand(cmd) {
		if c == nil {
			continue
		}
		if _, ok := c().(page.GlobalKeyMsg); ok {
			return true
		}
	}
	return false
}

// hint は Cmd の ChromeMsg からキー k のヒントを返す。
func hint(t *testing.T, cmd tea.Cmd, k string) (atom.Hint, bool) {
	t.Helper()

	for _, h := range chromeOf(t, cmd).Footer {
		if h.Key == k {
			return h, true
		}
	}
	return atom.Hint{}, false
}

// tab はペインを切り替え、**親へ差し戻さない**（差し戻すと 1 打鍵でタブも移る）。
func TestTabSwitchesPaneWithoutBubbling(t *testing.T) {
	m := sample(t, "a")
	if m.focus != focusList {
		t.Fatalf("初期のペイン = %v, want 一覧", m.focus)
	}

	next, cmd := step(t, m, pagetest.Press("tab"))
	if next.focus != focusBody {
		t.Errorf("tab の後のペイン = %v, want 本文", next.focus)
	}
	if bubbled(cmd) {
		t.Error("tab を親へ差し戻している（次のタブへ移ってしまう）")
	}
	if got := chromeOf(t, cmd).Status; !strings.Contains(got, paneBody) {
		t.Errorf("状態行 = %q, want 操作中のペインを含む", got)
	}

	next, _ = step(t, next, pagetest.Press("tab"))
	if next.focus != focusList {
		t.Errorf("2 度目の tab の後のペイン = %v, want 一覧", next.focus)
	}
}

// f は追従を切り替え、G は末尾へ戻して追従を再開する（screens.md の Logs タブ）。
func TestFollowToggleAndResume(t *testing.T) {
	m := sample(t, "a", "b", "c")
	m, _ = step(t, m, pagetest.Press("tab")) // 本文のペインへ

	m, _ = step(t, m, pagetest.Press("f"))
	if m.body.Following() {
		t.Fatal("f を押しても追従が続いている")
	}
	if got := m.header(); !strings.Contains(got, followOff) {
		t.Errorf("見出し = %q, want %q を含む", got, followOff)
	}

	m, _ = step(t, m, pagetest.Press("G"))
	if !m.body.Following() {
		t.Error("G で追従を再開できていない")
	}
}

// 手動スクロールで追従が切れる（末尾から離れた時点で切る）。
func TestManualScrollStopsFollowing(t *testing.T) {
	lines := make([]string, 0, 40)
	for range 40 {
		lines = append(lines, "line")
	}
	m := sample(t, lines...)
	m, _ = step(t, m, pagetest.Press("tab"))

	m, _ = step(t, m, pagetest.Press("k"))
	if m.body.Following() {
		t.Error("上へスクロールしても追従が続いている")
	}
}

// / でフィルタを入力し、enter で確定すると正規表現に一致する行だけが残る（FR-25）。
//
// **パターンにメタ文字を入れるのが要点である。** リテラルだけで組むと filterRegexp の
// regexp.Compile を strings.Contains 相当へ置き換えても全件緑のままで、受け入れ条件の
// 「**正規表現による**フィルタ」を何も縛らない。`^\[.*(ERROR|WARN)` は先頭一致・任意長・
// 選択の 3 つを同時に使うので、素朴な部分一致ではどの行も残らずに落ちる。
func TestFilterKeepsMatchingLinesOnly(t *testing.T) {
	m := sample(t, "info line", "[ERROR] boom", "[WARN] late", "WARN unbracketed")

	m, cmd := step(t, m, pagetest.Press("/"))
	if !m.body.Filtering() {
		t.Fatal("フィルタの入力モードに入っていない")
	}
	if got := chromeOf(t, cmd).Input; got != inputFilter {
		t.Errorf("入力中の名称 = %q, want %q", got, inputFilter)
	}

	for _, k := range strings.Split(`^\[.*(ERROR|WARN)`, "") {
		m, _ = step(t, m, pagetest.Press(k))
	}
	m, _ = step(t, m, pagetest.Press("enter"))

	got := m.body.View()
	// 残るのは選択のどちらの枝で一致した行も。落ちるのは非一致と、先頭一致（^\[）で外れる行。
	for _, want := range []string{"boom", "late"} {
		if !strings.Contains(got, want) {
			t.Errorf("一致する行 %q が消えている:\n%s", want, got)
		}
	}
	for _, ng := range []string{"info line", "unbracketed"} {
		if strings.Contains(got, ng) {
			t.Errorf("一致しない行 %q が残っている:\n%s", ng, got)
		}
	}
}

// esc は確定済みのフィルタを解除する（絞り込みの前の状態へ戻る）。
func TestEscClearsFilter(t *testing.T) {
	m := sample(t, "info line", "[ERROR] boom")
	m, _ = step(t, m, pagetest.Press("/"))
	for _, k := range strings.Split("ERROR", "") {
		m, _ = step(t, m, pagetest.Press(k))
	}
	m, _ = step(t, m, pagetest.Press("enter"))

	m, _ = step(t, m, pagetest.Press("esc"))
	if got := m.body.Filter(); got != "" {
		t.Fatalf("esc の後のフィルタ = %q, want 空", got)
	}
	if got := m.body.View(); !strings.Contains(got, "info line") {
		t.Errorf("解除しても行が戻っていない:\n%s", got)
	}
}

// 正規表現として解けないフィルタは理由を状態行に出す（黙って全行を消さない）。
func TestInvalidFilterReportsReason(t *testing.T) {
	m := sample(t, "info line")
	m, _ = step(t, m, pagetest.Press("/"))
	m, _ = step(t, m, pagetest.Press("["))
	m, cmd := step(t, m, pagetest.Press("enter"))

	if m.filterErr == nil {
		t.Fatal("不正な正規表現が理由として残っていない")
	}
	if got := chromeOf(t, cmd).Status; !strings.Contains(got, "正規表現") {
		t.Errorf("状態行 = %q, want 不正な正規表現である旨", got)
	}
	if got := m.body.View(); !strings.Contains(got, "info line") {
		t.Errorf("解けないフィルタで行が消えた:\n%s", got)
	}
}

// 入力中のグローバルキーは入力欄へ入り、親へ差し戻さない（screens.md の入力中）。
func TestFilterSwallowsGlobalKeys(t *testing.T) {
	m := sample(t, "a")
	m, _ = step(t, m, pagetest.Press("/"))

	m, cmd := step(t, m, pagetest.Press("q"))
	if bubbled(cmd) {
		t.Error("入力中のキーを親へ差し戻している（q で終了してしまう）")
	}
	m, _ = step(t, m, pagetest.Press("enter"))
	if got := m.body.Filter(); got != "q" {
		t.Errorf("確定したフィルタ = %q, want q", got)
	}
}

// journalctl が無い環境では J が縮退し、理由をフッタに出す（受け入れ条件）。
func TestJournalDegradesWithoutCapability(t *testing.T) {
	st, r := withLogs(t)
	st.Caps.Journal = false

	m := newTab(t, st)
	m.target = target{runner: r, file: dlogs.File{Name: "Worker_1.log"}, journal: false}

	next, cmd := step(t, m, pagetest.Press("J"))
	if next.target.journal {
		t.Error("journalctl が無いのに切り替わった")
	}
	h, ok := hint(t, cmd, "J")
	if !ok {
		t.Fatal("フッタに J が出ていない（キーは消さない）")
	}
	if h.Enabled || h.Reason != reasonNoJournal {
		t.Errorf("J のヒント = %v/%q, want false/%q", h.Enabled, h.Reason, reasonNoJournal)
	}
}

// systemd ユニットを持たない runner でも J は縮退する。
func TestJournalDegradesWithoutUnit(t *testing.T) {
	st, r := withLogs(t)
	r.UnitName = ""

	m := newTab(t, st)
	m.target = target{runner: r, file: dlogs.File{Name: "Worker_1.log"}, journal: false}

	next, cmd := step(t, m, pagetest.Press("J"))
	if next.target.journal {
		t.Error("ユニットが無いのに切り替わった")
	}
	h, _ := hint(t, cmd, "J")
	if h.Reason != reasonNoUnit {
		t.Errorf("J の理由 = %q, want %q", h.Reason, reasonNoUnit)
	}
}

// 一覧で enter を押すと、カーソル位置のログを本文に開く（フッタの `enter:開く`）。
//
// 前面に出た時点で 1 件目（最新の Worker ログ）が既に開いているので、`j` で 2 件目
// （Runner ログ）へ移してから押す。対象が差し替わるだけでなく、その本文が実際に
// 読み込まれるところまで見る（openSelected が nil を返しても対象は変わらないため）。
func TestEnterOpensSelectedLog(t *testing.T) {
	st, _ := withLogs(t)
	m := activated(t, st, 3)

	m, _ = step(t, m, pagetest.Press("j"))
	rows := m.tbl.Shown(sectionLogs)
	if len(rows) != 2 || m.target.file.Name == rows[1].file.Name {
		t.Fatalf("前提が崩れている（行数 %d / 対象 %q）", len(rows), m.target.file.Name)
	}

	next, cmd := step(t, m, pagetest.Press("enter"))
	if next.target.file.Name != rows[1].file.Name {
		t.Fatalf("enter の後の対象 = %q, want %q", next.target.file.Name, rows[1].file.Name)
	}
	next = pumpUntil(t, next, cmd, func(m Model) bool { return len(m.lines) >= 1 })
	if got := next.body.View(); !strings.Contains(got, "runner log") {
		t.Errorf("開いたログの本文が読み込まれていない:\n%s", got)
	}
}
