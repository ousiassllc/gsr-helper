package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/doctor/check"
	"github.com/ousiassllc/gsr-helper/internal/exec"
	"github.com/ousiassllc/gsr-helper/internal/runner"
	"github.com/ousiassllc/gsr-helper/internal/ui/discovery"
	"github.com/ousiassllc/gsr-helper/internal/ui/hostreq"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/pagetest"
)

// 件数の数え方は internal/ui/hostreq が、ヘッダと状態行の描き方は internal/ui/chrome が
// 検証する。ここで見るのは親 Model の経路だけ——いつ発行するか、届いた件数を
// chrome の入力へ写すか——である。

// discovered は検出が 1 周期終わった App と、そのとき返った Cmd を返す。
// err を渡すと期限切れ・失敗した周期になる（結果は取り込まれない）。
func discovered(a App, err error) (App, tea.Cmd) {
	return update(a, discovery.Msg{
		Result: runner.Result{Runners: []runner.Runner{sampleRunner()}},
		Err:    err,
	})
}

// takeHostReq は Cmd の束から前提チェックの結果を取り込む。無ければ ok が偽。
func takeHostReq(a App, cmd tea.Cmd) (App, bool) {
	msg, ok := pagetest.HostReqOf(cmd)
	if !ok {
		return a, false
	}
	a, _ = update(a, msg)
	return a, true
}

// 起動時の前提チェック（FR-44）の件数は chrome の入力へ写り、届き直せば更新される。
//
// 後半は Doctor タブでの再実行の経路である。sudo や docker グループを直して
// 再実行しても件数が据え置きだと、全て OK になったタブへ誘導し続けることになる。
func TestHostRequirementCountReachesChrome(t *testing.T) {
	a := newApp(exec.NewFake())

	a, _ = update(a, hostreq.Msg{Bad: 2})
	if got := a.chromeView().HostReq; got != 2 {
		t.Errorf("chrome へ写した件数 = %d, want 2", got)
	}
	if !strings.Contains(statusLine(a), "ホスト前提 2 件") {
		t.Errorf("状態行に件数が出ていない: %q", statusLine(a))
	}

	a, _ = update(a, hostreq.Msg{Bad: 0})
	if strings.Contains(statusLine(a), "ホスト前提") {
		t.Errorf("再実行の結果が届いても古い件数が残っている: %q", statusLine(a))
	}
}

// **検出に失敗した周期では発行しない。** 1 度きりの実行を空の runner 一覧で
// 使い切ってしまい、runner ごとに判定する 2 項目（NOPASSWD sudo / docker グループ
// 所属）がセッション中一度も走らず、警告も出ない（discover.go の applyDiscovered）。
func TestStartupHostRequirementWaitsForSuccessfulDiscovery(t *testing.T) {
	a := withHostChecks(newApp(exec.NewFake()), pagetest.StubCheck{Status: check.Fail})

	a, cmd := discovered(a, errTest)
	if _, ok := takeHostReq(a, cmd); ok {
		t.Fatal("検出に失敗した周期で前提チェックが発行された（空の runner 一覧で使い切る）")
	}

	a, cmd = discovered(a, nil)
	a, ok := takeHostReq(a, cmd)
	if !ok {
		t.Fatal("検出に成功した周期でも前提チェックが発行されない（FR-44 が走らない）")
	}
	if a.hostReq != 1 {
		t.Errorf("届いた件数 = %d, want 1", a.hostReq)
	}
}

// **再検出のたびには走らせない。** 判定対象はホストの構成であって秒単位で
// 変わらず、`sudo -l -U` は監査ログに記録されるため他のレコードを押し流す。
func TestStartupHostRequirementRunsOnce(t *testing.T) {
	a := withHostChecks(newApp(exec.NewFake()), pagetest.StubCheck{Status: check.Fail})

	a, cmd := discovered(a, nil)
	a, ok := takeHostReq(a, cmd)
	if !ok {
		t.Fatal("起動時の前提チェックが発行されていない")
	}

	for range 3 {
		var next tea.Cmd
		a, next = discovered(a, nil)
		if _, again := takeHostReq(a, next); again {
			t.Fatal("再検出のたびに前提チェックが走っている")
		}
	}
}
