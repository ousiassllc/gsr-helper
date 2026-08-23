package disk

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/disk"
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
	st.Result.Runners = []runner.Runner{busyRunner(t)}
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
	if !strings.Contains(body, dockerSkipReasonText) {
		t.Errorf("docker の SKIP 行が出ていない:\n%s", body)
	}
	if calls := fake.Calls(); len(calls) != 0 {
		t.Errorf("docker が使えないのに外部コマンドを発行している（%v）", calls)
	}
}

// docker が使える環境では内訳が行になり、先頭の「未使用リソース」だけを選択できる。
//
// **内訳（イメージ / ボリューム / …）は選べない。** 発行するのは
// docker system prune -f 1 本で種別を選り分けられず、しかも -f だけの prune は
// ボリュームを消さず dangling 以外のイメージも残すため、内訳を選ばせると確認
// ダイアログの解放見込みが**絶対に実現しない量**になる（disk.PruneReclaimable の doc）。
func TestDockerUsageBecomesSelectableRow(t *testing.T) {
	st, _ := dockerState()
	m := activate(t, newModel(t, st))

	body := m.View().Content
	if !strings.Contains(body, dockerCacheLabel) || !strings.Contains(body, dockerPruneLabel) {
		t.Fatalf("docker の内訳と未使用リソースの行が出ていない:\n%s", body)
	}
	if !strings.Contains(body, dockerBreakdownReason) {
		t.Errorf("内訳を選べない理由が行に出ていない:\n%s", body)
	}

	m, c := send(t, m, press("space"))
	if len(m.tbl.Checked()) != 1 {
		t.Fatal("未使用リソースの行を選択できない")
	}
	// 合計に載るのは prune -f が回収する種別（Containers / Build Cache）の Reclaimable
	// だけであり、回収されない Images / Local Volumes を含む素朴な合計（20.8G）ではない。
	if want := "選択: 1 件（合計 11.5G）"; !strings.Contains(c.Status, want) {
		t.Errorf("状態行 = %q, want %q を含む", c.Status, want)
	}

	// 次の行（内訳）へ移しても選べない。
	m, _ = send(t, m, press("j"))
	m, _ = send(t, m, press("space"))
	if n := len(m.tbl.Checked()); n != 1 {
		t.Errorf("内訳の行が選ばれている（選択 %d 件, want 1 件）", n)
	}
}

// FSStats の取得に失敗した状態では、要約行の使用率と inode を 0% ではなく「値なし」で
// 出す。**固定するのは page 側の配線（statsErr → FSSummaryView.Unavailable）である。**
// molecule の描画契約（TestFSSummaryLineUnavailable）は page が Unavailable を立て忘れ
// ても緑のままで、立て忘れると「使用 0%」になり**枯渇しているのに潤沢に見える**。
func TestFSStatsFailureShowsNoValueInSummary(t *testing.T) {
	st, _ := baseState()
	m := newModel(t, st)
	// 取得できた場合は使用率が出る（この検証が常に「値なし」を見ていない裏取り）。
	ok := disk.Stats{Path: "/", TotalBytes: 500, UsedBytes: 410, AvailBytes: 90, TotalInodes: 100, UsedInodes: 34, FreeInodes: 66}
	if body := update(t, m, fsStatsMsg{gen: m.gen, stats: ok, err: nil}).View().Content; !strings.Contains(body, "使用 82%") {
		t.Fatalf("取得できた使用率が要約行に出ていない（前提が崩れている）:\n%s", body)
	}
	body := update(t, m, fsStatsMsg{gen: m.gen, stats: disk.Stats{}, err: errors.New("statfs に失敗")}).View().Content
	if !strings.Contains(body, "使用 -") || !strings.Contains(body, "inode -") || strings.Contains(body, "使用 0%") {
		t.Errorf("取得に失敗した使用率と inode が「値なし」になっていない（0%% は枯渇を潤沢に見せる）:\n%s", body)
	}
}
