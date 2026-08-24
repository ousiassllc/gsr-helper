package runners_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/runner"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/pagetest"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/pagetest/cmdtest"
)

// ドレイン停止（FR-07）の検証を集める。

// ドレイン停止は確認ダイアログを経ず、待機画面を直接開く。
//
// 待機の開始にすぎず、待機中はいつでもキャンセルできるためである（screens.md の
// Runners タブの操作、Issue #5 の受け入れ条件）。**制約の注記は常に出す**。出さないと
// 利用者は「待てば必ず空く」と誤解して待ち続ける（FR-07 のドレイン停止の制約）。
func TestDrainOpensWaiterWithoutConfirm(t *testing.T) {
	st, f := opsState(sampleRunner("build01-1", true))
	m := newOpsModel(t, st)

	// 待機の Cmd は流さない。流すと待機がその場で終わってしまい、待機中の画面を
	// 見られない（対象に Runner.Worker が居ない環境では即座に停止まで進む）。
	next, _ := m.Update(press("d"))

	body := next.View().Content
	if strings.Contains(body, "実行しますか? [y/N]") {
		t.Fatalf("ドレイン停止で確認ダイアログが開いている:\n%s", body)
	}
	for _, want := range []string{
		"ドレイン停止中",
		"build01-1",
		"実行中のジョブの完了を待っています",
		"Worker PID 284193",
		"待機中も新しいジョブを受け付ける可能性があります",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("待機画面に %q が無い:\n%s", want, body)
		}
	}
	if got := issued(f); len(got) != 0 {
		t.Errorf("待機を始めた時点でコマンドが発行された: %v", got)
	}
}

// esc で待機をキャンセルでき、停止コマンドは発行されない。
//
// **キャンセルは待機を取り消すのであって、停止を指示するものではない**
// （svc.Drainer.Drain の doc）。ここで停止が走ると、ジョブを中断させないための
// 操作が中断そのものになる。
func TestDrainCancelIssuesNoStop(t *testing.T) {
	st, f := opsState(sampleRunner("build01-1", true))
	m := newOpsModel(t, st)

	// 待機の Cmd を手元に留めたまま esc を打つ。実際の並びと同じく、キャンセルの
	// あとに待機の Cmd が終わる（svc.Drain は ctx のキャンセルで戻る）状況を作る。
	next, drainCmd := m.Update(press("d"))
	canceled, cmd := next.Update(press("esc"))
	canceled = cmdtest.Advance(canceled, cmd, cmdtest.AdvanceRounds)

	// 留めておいた待機の Cmd をここで走らせる。キャンセル済みなので停止しない。
	if err := cmdtest.RunAll(drainCmd, cmdtest.CmdTimeout); err != nil {
		t.Fatalf("留めておいた待機の Cmd を流せない: %v", err)
	}

	if got := issued(f); len(got) != 0 {
		t.Errorf("キャンセルしたのにコマンドが発行された: %v", got)
	}
	c := opsChrome(t, canceled)
	if c.Modal {
		t.Error("esc で待機画面が閉じていない")
	}
	if !strings.Contains(c.Status, "キャンセル") {
		t.Errorf("状態行 = %q, キャンセルしたことを示していない", c.Status)
	}
}

// 待機が終われば停止コマンドを発行し、待機画面を閉じて結果を報告する。
//
// 対象に Runner.Worker が 1 つも残っていなければ、待機は最初の走査で終わる
// （svc.Drainer.Drain）。**走査は共有状態から来る**のでホストに依らない（pagetest.ScanOf）。
// 既定値固定の svc.Drain を呼んでいたころは停止条件が実ホストの /proc で決まり、worker が
// 居るホストでは待ち時間が無制限（FR-07）である以上待機が終わらなかった（Issue #155）。
// **走査の回数が 1 でなければ落とす**のはそのためである（busy でない対象は 1 回で終わる）。
func TestDrainStopsAfterWaiting(t *testing.T) {
	st, f := opsState(sampleRunner("build01-1", false))
	scans := 0
	base := st.ScanProcs
	st.ScanProcs = func() ([]runner.Process, error) { scans++; return base() }
	m := newOpsModel(t, st)

	m = opsSend(t, m, "d")

	if scans != 1 {
		t.Errorf("走査の回数 = %d, want 1（0 なら停止条件がホストのプロセス表に依存している）", scans)
	}
	want := []string{"systemctl stop " + unit1}
	if got := issued(f); !reflect.DeepEqual(got, want) {
		t.Fatalf("発行コマンド = %v, want %v", got, want)
	}
	c := opsChrome(t, m)
	if c.Modal {
		t.Error("待機が終わったのに待機画面が閉じていない")
	}
	if !strings.Contains(c.Status, "ドレイン停止: 1 件成功") {
		t.Errorf("状態行 = %q, want ドレイン停止: 1 件成功", c.Status)
	}
}

// 対象が複数のときは順に待機し、全件を停止する。
//
// 同時に走らせると、待機画面 1 枚ではどの runner を待っているのかを表せない。
func TestDrainRunsTargetsInOrder(t *testing.T) {
	st, f := opsState()
	m := newOpsModel(t, st)

	m = opsSend(t, m, "space", "j", "space", "d")

	want := []string{"systemctl stop " + unit1, "systemctl stop " + unit2}
	if got := issued(f); !reflect.DeepEqual(got, want) {
		t.Errorf("発行コマンド = %v, want %v", got, want)
	}
	if got := opsChrome(t, m).Status; !strings.Contains(got, "ドレイン停止: 2 件成功") {
		t.Errorf("状態行 = %q, want ドレイン停止: 2 件成功", got)
	}
}

// 対象が複数のときは待機画面の見出しに進捗（(1/2)）を出す。
//
// **モーダルは 1 枚しか無い。** 何件中の何件目を待っているのかを見出しに出さないと、
// 一括ドレインの途中で「もう終わったのか、次の対象を待っているのか」が読めない。
//
// 1 件のときは出さない（分母のある表示は順に処理していることの合図であり、
// 1 件しか無いのに出すと他にも対象があると読める）。
func TestDrainWaiterTitleShowsProgress(t *testing.T) {
	tests := map[string]struct {
		keys []string
		want string
	}{
		"2 件なら進捗を出す": {[]string{"space", "j", "space", "d"}, "(1/2)"},
		"1 件なら出さない":  {[]string{"d"}, ""},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			st, _ := opsState()
			m := newOpsModel(t, st)

			m = opsSend(t, m, tt.keys[:len(tt.keys)-1]...)
			// 待機の Cmd は流さない（流すと 1 件目が終わって次の見出しになる）。
			opened, _ := m.Update(press("d"))

			body := opened.View().Content
			switch {
			case tt.want == "" && strings.Contains(body, "(1/"):
				t.Errorf("1 件なのに進捗が出ている:\n%s", body)
			case tt.want != "" && !strings.Contains(body, tt.want):
				t.Errorf("見出しに %q が無い:\n%s", tt.want, body)
			}
		})
	}
}

// 待機中もモーダル表示中の規則が働き、グローバルキーは親へ差し戻されない。
func TestDrainWaiterSwallowsGlobalKeys(t *testing.T) {
	st, _ := opsState(sampleRunner("build01-1", true))
	m := newOpsModel(t, st)
	next, _ := m.Update(press("d"))

	for _, k := range []string{"q", "1"} {
		_, cmd := next.Update(press(k))
		c, global, bubbled := pagetest.ScanKey(cmd)
		if bubbled {
			t.Errorf("待機中の %q が親へ差し戻された（%v）", k, global.Press)
		}
		if !c.Modal {
			t.Errorf("待機中の %q で待機画面が閉じた", k)
		}
	}
}

// 待機中の表示は 3 秒ごとの共有状態で引き直される（screens.md の詳細画面と同じ規則）。
//
// **svc.Drain の progress コールバックに依存しない。** コールバックは別の goroutine
// から呼ばれ、そこから tea.Model を触ると競合する。
func TestDrainWaiterFollowsStateUpdates(t *testing.T) {
	st, _ := opsState(sampleRunner("build01-1", true))
	m := newOpsModel(t, st)
	next, _ := m.Update(press("d"))

	// 同じ runner のジョブが 1 件終わった周期を配る。
	updated, _ := opsState(sampleRunner("build01-1", false))
	after, _ := next.Update(updated)

	if body := after.View().Content; strings.Contains(body, "Worker PID 284193") {
		t.Errorf("待機画面が古い対象ジョブを出したままである:\n%s", body)
	}
}
