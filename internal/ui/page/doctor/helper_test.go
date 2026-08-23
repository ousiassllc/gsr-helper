package doctor

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	dom "github.com/ousiassllc/gsr-helper/internal/doctor"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/pagetest"
)

// 内部テスト（package doctor）にしてあるのは、診断の完了を表す doneMsg を
// 直に配るためである。**本番へテスト専用の口を開けない**ためにこの形を採る
// （page/setup の formvalues_test.go と同じ置き方）。
//
// 実際の診断（doctor.Run）は走らせない。実ホストの docker や systemd の有無で
// 結果が変わり、CI と手元で判定が食い違う。**このタブの検証対象は結果の取り込みと
// 画面の応答**であり、判定そのものは internal/doctor 側で検査してある。

// tabIndex は検証で使うタブ番号。ChromeMsg と page.Do の突き合わせに使う。
const tabIndex = 4

// newPage は共有状態を配っただけのタブを返す。**診断はまだ始まっていない。**
func newPage(t *testing.T) Model {
	t.Helper()

	m := New(tabIndex, pagetest.State(80, 20))
	next, _ := m.Update(pagetest.State(80, 20))
	return asModel(t, next)
}

// activated は共有状態を配ったうえでタブを前面に出したタブと、そのとき返った
// Cmd を返す。診断はこの時点で始まる。
func activated(t *testing.T) (Model, tea.Cmd) {
	t.Helper()
	return send(t, newPage(t), page.ActivateMsg{})
}

// asModel は tea.Model を具体型へ戻す。
func asModel(t *testing.T, m tea.Model) Model {
	t.Helper()

	got, ok := m.(Model)
	if !ok {
		t.Fatalf("Model の型 = %T, want doctor.Model", m)
	}
	return got
}

// send は Msg を配って次の Model と Cmd を返す。
func send(t *testing.T, m Model, msg tea.Msg) (Model, tea.Cmd) {
	t.Helper()

	next, cmd := m.Update(msg)
	return asModel(t, next), cmd
}

// press はキーを打って次の Model と Cmd を返す。
func press(t *testing.T, m Model, k string) (Model, tea.Cmd) {
	t.Helper()
	return send(t, m, pagetest.Press(k))
}

// result は診断結果 1 件を組み立てる。
func result(id, category, target string, st dom.Status) dom.CheckResult {
	return dom.CheckResult{
		ID: id, Category: category, Target: target, Status: st,
		Summary: id + " の要約", Detail: id + " の検出内容",
		Impact: id + " の影響", Remedy: id + " の対処", Startup: false,
	}
}

// finishedAt は診断の完了時刻。見出しの検証に使う。
var finishedAt = time.Date(2026, 8, 23, 12, 6, 20, 0, time.UTC)

// deliver は診断が終わった状態を作る。
func deliver(t *testing.T, m Model, rs []dom.CheckResult) (Model, tea.Cmd) {
	t.Helper()
	return send(t, m, doneMsg{id: "", results: dom.Sort(rs), at: finishedAt})
}

// chromeOf は Cmd の束から ChromeMsg を取り出す。
func chromeOf(t *testing.T, cmd tea.Cmd) page.ChromeMsg {
	t.Helper()

	got, ok := pagetest.ChromeOf(cmd)
	if !ok {
		t.Fatal("ChromeMsg が発行されていない（親が状態行とフッタを更新できない）")
	}
	return got
}

// hintFor はフッタから指定したキーのヒントを返す。
func hintFor(t *testing.T, c page.ChromeMsg, k string) (enabled bool, desc string) {
	t.Helper()

	for _, h := range c.Footer {
		if h.Key == k {
			return h.Enabled, h.Desc
		}
	}
	t.Fatalf("フッタにキー %q が無い（%+v）", k, c.Footer)
	return false, ""
}

// sample は一覧に流し込む代表的な診断結果を返す。
func sample() []dom.CheckResult {
	return []dom.CheckResult{
		result("authz.hidepid", dom.Categories()[0], "", dom.Warn),
		result("job.dockergroup", "ジョブ実行の前提", "build01", dom.Fail),
		result("job.docker", "ジョブ実行の前提", "", dom.OK),
		result("docker.daemon", "docker", "", dom.Skip),
	}
}
