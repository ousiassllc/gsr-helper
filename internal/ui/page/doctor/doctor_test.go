package doctor

import (
	"strings"
	"testing"

	dom "github.com/ousiassllc/gsr-helper/internal/doctor"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/pagetest"
)

// 最初の共有状態で診断が始まる（FR-32）。タブを開いた時点で結果が要るためである。
func TestFirstStateStartsDiagnostics(t *testing.T) {
	t.Parallel()

	m, cmd := newPage(t)
	if !m.running {
		t.Error("running = false, want true（最初の共有状態で診断が始まらない）")
	}
	if c := chromeOf(t, cmd); !strings.Contains(c.Status, "実行中") {
		t.Errorf("状態行 = %q, want 実行中の表示", c.Status)
	}
}

// **共有状態は 3 秒ごとに全タブへ配られる。** 2 回目以降で診断を始めると、
// `sudo -l -U` と `journalctl -k` が 3 秒ごとに監査ログへ積み上がる。
func TestSubsequentStateDoesNotRestartDiagnostics(t *testing.T) {
	t.Parallel()

	m, _ := newPage(t)
	m, _ = deliver(t, m, sample())
	if m.running {
		t.Fatal("診断が終わっていない")
	}

	m, _ = send(t, m, pagetest.State(80, 20))
	if m.running {
		t.Error("2 回目の共有状態で診断が再開した（3 秒ごとに走り続ける）")
	}
}

// 結果が届いたら一覧と件数の見出しが更新される。
func TestDoneUpdatesRowsAndSummary(t *testing.T) {
	t.Parallel()

	m, _ := newPage(t)
	m, _ = deliver(t, m, sample())

	if got := m.summary; got.OK != 1 || got.Warn != 1 || got.Fail != 1 || got.Skip != 1 {
		t.Errorf("件数 = %+v, want OK/WARN/FAIL/SKIP それぞれ 1", got)
	}
	if n := len(m.tbl.Shown(sectionResults)); n != 4 {
		t.Errorf("一覧の行数 = %d, want 4", n)
	}

	view := m.View().Content
	for _, want := range []string{"OK 1", "WARN 1", "FAIL 1", "SKIP 1", "最終実行: 12:06:20"} {
		if !strings.Contains(view, want) {
			t.Errorf("見出しに %q が無い:\n%s", want, view)
		}
	}
}

// r は全項目を再実行し、**同時に親へも差し戻す**（Disk タブと同じ扱い）。
// 差し戻さないと Doctor タブに居る間だけ runner の再検出が止まる。
func TestRefreshRerunsAndBubbles(t *testing.T) {
	t.Parallel()

	m, _ := newPage(t)
	m, _ = deliver(t, m, sample())

	next, cmd := press(t, m, "r")
	if !next.running {
		t.Error("running = false, want true（r で再実行が始まらない）")
	}
	if _, _, ok := pagetest.ScanKey(cmd); !ok {
		t.Error("r が親へ差し戻されていない（runner の再検出が止まる）")
	}
}

// 実行中の r は重ねない。二重に走らせると同じコマンドが並行して発行される。
func TestRefreshWhileRunningIsIgnored(t *testing.T) {
	t.Parallel()

	m, _ := newPage(t) // 最初の診断が走ったまま
	if !m.running {
		t.Fatal("診断が始まっていない")
	}

	_, cmd := press(t, m, "r")
	c := chromeOf(t, cmd)
	enabled, _ := hintFor(t, c, "r")
	if enabled {
		t.Error("実行中なのに再実行が有効になっている")
	}
}

// enter で詳細（検出内容・影響・推奨する対処）を開く（FR-33）。
func TestEnterOpensDetail(t *testing.T) {
	t.Parallel()

	m, _ := newPage(t)
	m, _ = deliver(t, m, sample())

	next, cmd := press(t, m, "enter")
	if !next.overlay.Active() {
		t.Fatal("詳細画面が開いていない")
	}
	if c := chromeOf(t, cmd); !c.Modal {
		t.Error("ChromeMsg.Modal = false, want true（親がモーダル表示を知れない）")
	}
}

// **モーダル表示中は背後の一覧へキーが流れず、親へも差し戻さない。**
// グローバルキーを閉じ込められるのはこの判定を持つ page だけである。
func TestKeysDoNotLeakWhileModalIsOpen(t *testing.T) {
	t.Parallel()

	m, _ := newPage(t)
	m, _ = deliver(t, m, sample())
	m, _ = press(t, m, "enter")
	if !m.overlay.Active() {
		t.Fatal("詳細画面が開いていない")
	}

	before := m.tbl.FilterValue()
	next, cmd := press(t, m, "/")
	if _, _, ok := pagetest.ScanKey(cmd); ok {
		t.Error("モーダル表示中のキーが親へ差し戻された")
	}
	if next.tbl.Filtering() || next.tbl.FilterValue() != before {
		t.Error("モーダル表示中のキーが背後の一覧へ流れた（絞り込みが始まっている）")
	}
}

// 詳細画面の r はその 1 項目だけを再実行する（FR-34 の個別再実行）。
func TestDetailRecheckRunsSingleCheck(t *testing.T) {
	t.Parallel()

	m, _ := newPage(t)
	m, _ = deliver(t, m, []dom.CheckResult{
		result("job.dockergroup", "ジョブ実行の前提", "build01", dom.Fail),
	})

	next, cmd := send(t, m, page.ResultMsg{Kind: Kind, Msg: RecheckMsg{ID: "job.dockergroup"}})
	if !next.running {
		t.Error("running = false, want true（個別の再実行が始まらない）")
	}
	if cmd == nil {
		t.Error("再実行の Cmd が発行されていない")
	}
}

// 知らない項目の再実行は何も起こさない（レジストリに無い識別子）。
func TestDetailRecheckWithUnknownIDDoesNothing(t *testing.T) {
	t.Parallel()

	m, _ := newPage(t)
	m, _ = deliver(t, m, sample())

	next, _ := send(t, m, page.ResultMsg{Kind: Kind, Msg: RecheckMsg{ID: "存在しない項目"}})
	if next.running {
		// running は立つが Cmd は nil。実行中表示が戻らなくなるほうが害が大きいので
		// 立てない設計にしてある。
		t.Error("知らない識別子で実行中の表示が立ちっぱなしになる")
	}
}

// 個別の再実行はその項目の行だけを差し替える。
func TestPartialResultReplacesOnlyThatCheck(t *testing.T) {
	t.Parallel()

	m, _ := newPage(t)
	m, _ = deliver(t, m, sample())

	fixed := result("job.dockergroup", "ジョブ実行の前提", "build01", dom.OK)
	m, _ = send(t, m, doneMsg{
		id:      "job.dockergroup",
		results: []dom.CheckResult{fixed},
		at:      finishedAt,
	})

	if n := len(m.results); n != 4 {
		t.Fatalf("結果の件数 = %d, want 4（他の項目まで消えている）", n)
	}
	for _, r := range m.results {
		if r.ID == "job.dockergroup" && r.Status != dom.OK {
			t.Errorf("再実行した項目の判定 = %v, want %v", r.Status, dom.OK)
		}
	}
	if m.summary.Fail != 0 {
		t.Errorf("FAIL の件数 = %d, want 0（件数が数え直されていない）", m.summary.Fail)
	}
}

// フッタは enter と r を出す。行が無ければ enter は無効にする
// （押しても何も起きないキーを有効に見せない。設計原則 2）。
func TestFooter(t *testing.T) {
	t.Parallel()

	m, _ := newPage(t)
	m, cmd := deliver(t, m, sample())

	c := chromeOf(t, cmd)
	if enabled, desc := hintFor(t, c, "enter"); !enabled || desc != detailDesc {
		t.Errorf("enter のヒント = %v/%q, want true/%q", enabled, desc, detailDesc)
	}
	if enabled, desc := hintFor(t, c, "r"); !enabled || desc != rerunDesc {
		t.Errorf("r のヒント = %v/%q, want true/%q", enabled, desc, rerunDesc)
	}

	empty, emptyCmd := deliver(t, m, nil)
	if enabled, _ := hintFor(t, chromeOf(t, emptyCmd), "enter"); enabled {
		t.Error("行が無いのに enter が有効になっている")
	}
	if got := empty.View().Content; !strings.Contains(got, noResultMessage) {
		t.Errorf("結果が無いときの表示に %q が無い:\n%s", noResultMessage, got)
	}
}

// 絞り込みは分類・要約・対象のいずれかに当たれば残す。
func TestFilterMatchesCategorySummaryAndTarget(t *testing.T) {
	t.Parallel()

	rs := []dom.CheckResult{
		result("job.dockergroup", "ジョブ実行の前提", "build01", dom.Fail),
		result("net.reach", "ネットワーク", "", dom.OK),
	}
	tests := map[string]struct {
		query string
		want  int
	}{
		"分類で絞る": {query: "ネットワーク", want: 1},
		"対象で絞る": {query: "build01", want: 1},
		"要約で絞る": {query: "net.reach", want: 1},
		"当たらない": {query: "該当なし", want: 0},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			m, _ := newPage(t)
			m, _ = deliver(t, m, rs)
			m, _ = press(t, m, "/")
			for _, r := range tt.query {
				m, _ = press(t, m, string(r))
			}
			m, _ = press(t, m, "enter")

			if got := len(m.tbl.Shown(sectionResults)); got != tt.want {
				t.Errorf("絞り込み後の行数 = %d, want %d", got, tt.want)
			}
		})
	}
}

// 入力中は状態行がそれを最優先で示す（グローバルキーが効かない状態そのもの）。
func TestStatusShowsFilteringFirst(t *testing.T) {
	t.Parallel()

	m, _ := newPage(t)
	m, _ = deliver(t, m, sample())
	_, cmd := press(t, m, "/")

	// 絞り込みの開始は default の経路を通るため、ChromeMsg は入れ子の Cmd の
	// 奥にある。ScanKey は束を辿って拾う。
	c, _, _ := pagetest.ScanKey(cmd)
	if c.Input != inputFilter {
		t.Errorf("ChromeMsg.Input = %q, want %q", c.Input, inputFilter)
	}
}
