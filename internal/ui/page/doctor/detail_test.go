package doctor

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	dom "github.com/ousiassllc/gsr-helper/internal/doctor"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/pagetest"
)

// 詳細画面（FR-33）と、そこからの個別再実行（FR-34）を検証する。
//
// モーダルを page.Overlay に載せずに直接組み立てる検証を含むのは、個別再実行の
// 唯一の入口が r のキーハンドラ（detail.go の handleKey）だからである。決定
// （page.ResultMsg）を page へ直に注入する検証だけでは、その入口を迂回してしまう。

// detailModal は 1 件の結果を出した状態の詳細画面を返す。
func detailModal(t *testing.T, r dom.CheckResult) modal {
	t.Helper()

	const w, h = 60, 20
	next, _ := newModal(pagetest.State(w, h)).Model.Update(page.AttachMsg{Tab: tabIndex})
	next, _ = next.Update(page.SizeMsg{W: w, H: h})
	next, _ = next.Update(OpenMsg{Result: r})

	got, ok := next.(modal)
	if !ok {
		t.Fatalf("Model の型 = %T, want modal", next)
	}
	return got
}

// recheckOf は Cmd から個別再実行の決定を取り出す。無ければ ok が偽。
//
// 決定は page.Do に包まれて発行元のタブへ戻る。包みを解かずに型だけを見ると、
// 宛先のタブ番号が抜けていても緑になる。
func recheckOf(t *testing.T, cmd tea.Cmd) (RecheckMsg, bool) {
	t.Helper()

	for _, msg := range pagetest.Msgs(cmd) {
		tab, ok := msg.(page.TabMsg)
		if !ok {
			continue
		}
		if tab.Tab != tabIndex {
			t.Errorf("宛先のタブ番号 = %d, want %d", tab.Tab, tabIndex)
		}
		res, ok := tab.Msg.(page.ResultMsg)
		if !ok {
			continue
		}
		if re, ok := res.Msg.(RecheckMsg); ok {
			return re, true
		}
	}
	return RecheckMsg{}, false
}

// 詳細画面の r はその 1 項目だけの再実行を page へ返す（FR-34 の個別再実行）。
//
// **項目が定まっていなければ何も返さない。** 開く前の詳細画面（結果が空）で r を
// 押すと、レジストリに無い識別子で再実行が始まり「診断を実行中です…」が戻らない。
func TestDetailRecheckKey(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		result dom.CheckResult
		wantID string
		want   bool
	}{
		"項目があれば再実行を返す": {
			result: result("job.dockergroup", "ジョブ実行の前提", "build01", dom.Fail),
			wantID: "job.dockergroup",
			want:   true,
		},
		"項目が無ければ返さない": {result: dom.CheckResult{}, wantID: "", want: false},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			_, cmd := detailModal(t, tt.result).Update(pagetest.Press("r"))
			re, ok := recheckOf(t, cmd)
			if ok != tt.want {
				t.Fatalf("再実行の決定 = %v, want %v", ok, tt.want)
			}
			if ok && re.ID != tt.wantID {
				t.Errorf("再実行する項目 = %q, want %q", re.ID, tt.wantID)
			}
		})
	}
}

// 詳細画面は 検出内容 / 影響 / 推奨する対処 の 3 節を出す（FR-33）。
//
// **中身の無い節は出さない。** OK と SKIP には影響も対処も無く、空の見出しだけが
// 並ぶと「対処が要るのに書かれていない」と読める。
func TestDetailSections(t *testing.T) {
	t.Parallel()

	bad := result("job.dockergroup", "ジョブ実行の前提", "build01", dom.Fail)
	clean := func(st dom.Status) dom.CheckResult {
		r := result("job.docker", "ジョブ実行の前提", "", st)
		r.Impact, r.Remedy = "", ""
		return r
	}

	tests := map[string]struct {
		result dom.CheckResult
		want   []string
		absent []string
	}{
		"FAIL は 3 節すべて出す": {
			result: bad,
			want:   []string{labelDetail, bad.Detail, labelImpact, bad.Impact, labelRemedy, bad.Remedy},
			absent: nil,
		},
		"OK は検出内容だけ": {
			result: clean(dom.OK),
			want:   []string{labelDetail},
			absent: []string{labelImpact, labelRemedy},
		},
		"SKIP も検出内容だけ": {
			result: clean(dom.Skip),
			want:   []string{labelDetail},
			absent: []string{labelImpact, labelRemedy},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got := detailModal(t, tt.result).View().Content
			for _, want := range tt.want {
				if !strings.Contains(got, want) {
					t.Errorf("詳細画面に %q が無い:\n%s", want, got)
				}
			}
			for _, absent := range tt.absent {
				if strings.Contains(got, absent) {
					t.Errorf("中身の無い節 %q が出ている:\n%s", absent, got)
				}
			}
		})
	}
}

// 複数行の対処は改行を保つ。詰めると貼り付けたときに動かないものになる。
func TestDetailKeepsRemedyLineBreaks(t *testing.T) {
	t.Parallel()

	r := result("job.dockergroup", "ジョブ実行の前提", "build01", dom.Fail)
	first, second := "sudo usermod -aG docker runner", "sudo systemctl restart docker"
	r.Remedy = first + "\n" + second

	got := detailModal(t, r).View().Content
	var found []string
	for _, line := range strings.Split(got, "\n") {
		if trimmed := strings.TrimSpace(line); trimmed == first || trimmed == second {
			found = append(found, trimmed)
		}
	}
	if len(found) != 2 || found[0] != first || found[1] != second {
		t.Errorf("対処の 2 行が別々の行として出ていない（%q）:\n%s", found, got)
	}
}

// 個別再実行の結果が届いたら、開いたままの詳細画面を新しい内容へ差し替える（FR-34）。
//
// 差し替えないと、対処を終えて r を押した運用者が古い 検出内容 / 影響 / 推奨する
// 対処 を見続ける。受け入れ条件「対処後に個別または全体を再実行できる」が実効を失う。
func TestRecheckRefreshesOpenDetail(t *testing.T) {
	t.Parallel()

	stale := result("job.dockergroup", "ジョブ実行の前提", "build01", dom.Fail)
	m, _ := activated(t)
	m, _ = deliver(t, m, []dom.CheckResult{stale})
	m, _ = press(t, m, "enter")
	if !m.overlay.Active() {
		t.Fatal("詳細画面が開いていない")
	}
	if got := m.View().Content; !strings.Contains(got, stale.Detail) {
		t.Fatalf("詳細画面に元の検出内容が出ていない:\n%s", got)
	}

	fixed := stale
	fixed.Status, fixed.Detail = dom.OK, "docker グループに所属しています"
	fixed.Impact, fixed.Remedy = "", ""
	m, _ = send(t, m, doneMsg{id: fixed.ID, results: []dom.CheckResult{fixed}, at: finishedAt})

	got := m.View().Content
	if !m.overlay.Active() {
		t.Fatalf("再実行で詳細画面が閉じてしまった:\n%s", got)
	}
	if !strings.Contains(got, fixed.Detail) {
		t.Errorf("詳細画面が新しい検出内容へ差し替わっていない:\n%s", got)
	}
	if strings.Contains(got, stale.Detail) {
		t.Errorf("古い検出内容が残っている:\n%s", got)
	}
}

// 対象が違う行の再実行では差し替えない。
//
// runner ごとに判定する項目は 1 つの識別子で runner の数だけ行を返すため、識別子
// だけで探すと別の runner の結果が今見ている詳細に化けて出る。
func TestRecheckKeepsDetailOfAnotherTarget(t *testing.T) {
	t.Parallel()

	other := result("job.dockergroup", "ジョブ実行の前提", "build01-1", dom.Fail)
	shown := result("job.dockergroup", "ジョブ実行の前提", "build01-2", dom.Fail)
	shown.Detail = "build01-2 は所属していません"

	// 2 行目（別の対象）の詳細を開く。1 行目を再実行しても差し替わらないことを見る。
	m, _ := activated(t)
	m, _ = deliver(t, m, []dom.CheckResult{other, shown})
	m, _ = press(t, m, "down")
	m, _ = press(t, m, "enter")
	if !strings.Contains(m.View().Content, shown.Detail) {
		t.Fatalf("2 行目の詳細が開いていない:\n%s", m.View().Content)
	}

	fixed := other
	fixed.Status, fixed.Detail = dom.OK, "build01-1 は所属しています"
	m, _ = send(t, m, doneMsg{
		id:      fixed.ID,
		results: []dom.CheckResult{fixed, shown},
		at:      finishedAt,
	})

	got := m.View().Content
	if strings.Contains(got, fixed.Detail) {
		t.Errorf("別の対象の結果へ差し替わっている:\n%s", got)
	}
	if !strings.Contains(got, shown.Detail) {
		t.Errorf("開いていた対象の詳細が消えている:\n%s", got)
	}
}
