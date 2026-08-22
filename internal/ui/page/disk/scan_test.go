package disk

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/runner"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
)

// 集計の寿命（FR-27 / FR-28）を固定する。

// 前面に出た時点で集計が始まり、判明した行から順に表へ入る（FR-28）。
//
// 全件そろってから描く形にすると、大きな runner が 1 台あるだけで画面が数分固まる。
// 並べ替えないのは、判明するたびに行が入れ替わると選びかけの対象を目で追えなくなる
// ためである。
func TestActivateStartsScanAndFillsRowsInArrivalOrder(t *testing.T) {
	st, _ := baseState()
	m := update(t, newModel(t, st), page.ActivateMsg{})
	if m.scan == nil {
		t.Fatal("page.ActivateMsg で集計が始まっていない")
	}

	// docker が使えないので SKIP 行が 1 行だけ載った状態から始まる。
	gen, base := m.gen, len(m.tbl.Shown(sectionTargets))
	m = update(t, m, usageMsg{gen: gen, usage: fakeUsage("build01-1 / _work/zzz", 100), ok: true})
	if got := len(m.tbl.Shown(sectionTargets)); got != base+1 {
		t.Fatalf("1 件目が表に入っていない（%d 行, want %d 行）", got, base+1)
	}
	m = update(t, m, usageMsg{gen: gen, usage: fakeUsage("build01-1 / _work/aaa", 200), ok: true})

	body := m.View().Content
	first, second := strings.Index(body, "_work/zzz"), strings.Index(body, "_work/aaa")
	if first < 0 || second < 0 {
		t.Fatalf("判明した行が表に出ていない:\n%s", body)
	}
	if first > second {
		t.Errorf("判明順ではなく別の順で並んでいる（容量順に並べ替えている）:\n%s", body)
	}
}

// 裏へ回る・終了するときに集計を畳み、以降の結果を取り込まない。
//
// 畳む機会が無いとタブを行き来するたびに走査が積み上がる。**表の中身と選択は
// 捨てない**（裏に回ったことが利用者に見えてしまう。page.DeactivateMsg の doc）。
func TestScanStopsOnDeactivateAndShutdown(t *testing.T) {
	for name, stop := range map[string]tea.Msg{
		"DeactivateMsg": page.DeactivateMsg{},
		"ShutdownMsg":   page.ShutdownMsg{},
	} {
		t.Run(name, func(t *testing.T) {
			st, _ := baseState()
			m := update(t, newModel(t, st), page.ActivateMsg{})
			gen := m.gen
			m = update(t, m, usageMsg{gen: gen, usage: fakeUsage("build01-1 / _work/bar", 100), ok: true})

			m, _ = send(t, m, stop)
			if m.scan != nil {
				t.Fatalf("%s で集計が畳まれていない", name)
			}

			// 打ち切りが届くまでに送られた結果は捨てる。
			m = update(t, m, usageMsg{gen: gen, usage: fakeUsage("build01-1 / _work/late", 100), ok: true})
			if strings.Contains(m.View().Content, "_work/late") {
				t.Error("畳んだ集計の結果が表に入っている")
			}
			if !strings.Contains(m.View().Content, "_work/bar") {
				t.Error("裏へ回ったときに表の中身を捨てている")
			}
		})
	}
}

// 張り直した集計に、前の集計の残りが混ざらない。
//
// 打ち切りは即座には届かない。世代を突き合わせないと、新しい表に古い結果が入り、
// 同じ対象が 2 行並ぶ（識別子は同じなので選択も取り合う）。
func TestStaleScanResultIsDropped(t *testing.T) {
	st, _ := baseState()
	m := update(t, newModel(t, st), page.ActivateMsg{})
	stale := m.gen

	m, _ = send(t, m, page.DeactivateMsg{})
	m = update(t, m, page.ActivateMsg{})
	if m.gen == stale {
		t.Fatal("張り直しで世代が進んでいない")
	}

	m = update(t, m, usageMsg{gen: stale, usage: fakeUsage("build01-1 / _work/old", 100), ok: true})
	if strings.Contains(m.View().Content, "_work/old") {
		t.Error("前の集計の結果が新しい表に入っている")
	}
}

// ジョブ実行中 runner の _work は選択できず、理由が行に出る（FR-31）。
func TestBusyWorkTargetIsNotSelectable(t *testing.T) {
	st, _ := baseState()
	st.Result.Runners = []runner.Runner{tempRunner(t, true)}
	m := activate(t, newModel(t, st))

	body := m.View().Content
	if !strings.Contains(body, "_work/bar") {
		t.Fatalf("集計対象の行が出ていない（前提が崩れている）:\n%s", body)
	}
	if !strings.Contains(body, busyReasonPrefix) {
		t.Errorf("選択できない理由が行に出ていない:\n%s", body)
	}

	// カーソルを _work の行（docker の SKIP 行の次）へ移してから選ぶ。
	m, _ = send(t, m, press("j"))
	m, c := send(t, m, press("space"))
	if n := len(m.tbl.Checked()); n != 0 {
		t.Errorf("選択できない行が選ばれている（%d 件）", n)
	}
	if c.Status != "" {
		t.Errorf("選択件数が出ている（status = %q）", c.Status)
	}
}

// docker が使えない環境では docker の行が SKIP になり、docker を 1 度も起動しない。
func TestDockerDegradesToSkipRow(t *testing.T) {
	st, fake := baseState()
	m := activate(t, newModel(t, st))

	body := m.View().Content
	if !strings.Contains(body, dockerSkipPrefix) {
		t.Errorf("docker の SKIP 行が出ていない:\n%s", body)
	}
	if calls := fake.Calls(); len(calls) != 0 {
		t.Errorf("docker が使えないのに外部コマンドを発行している（%v）", calls)
	}
}

// docker が使える環境では内訳が行になり、選択できる。
func TestDockerUsageBecomesSelectableRow(t *testing.T) {
	st, _ := dockerState()
	m := activate(t, newModel(t, st))

	if !strings.Contains(m.View().Content, dockerCacheLabel) {
		t.Fatalf("docker の内訳が行になっていない:\n%s", m.View().Content)
	}
	m, c := send(t, m, press("space"))
	if len(m.tbl.Checked()) != 1 {
		t.Fatal("docker の行を選択できない")
	}
	if !strings.Contains(c.Status, "選択: 1 件") {
		t.Errorf("選択件数が状態行に出ていない（status = %q）", c.Status)
	}
}
