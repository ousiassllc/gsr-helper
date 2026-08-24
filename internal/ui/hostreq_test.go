package ui

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/doctor/check"
	"github.com/ousiassllc/gsr-helper/internal/exec"
	"github.com/ousiassllc/gsr-helper/internal/ui/hostreq"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/pagetest"
)

// 件数の数え方は internal/ui/hostreq が、ヘッダと状態行の描き方は internal/ui/chrome が
// 検証する。ここで見るのは親 Model の経路だけ——いつ発行するか、届いた件数を
// chrome の入力へ写すか——である。

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

// 起動時の前提チェック（FR-44）は、検出に成功した最初の周期でだけ発行する。
//
// **検出に失敗した周期では発行しない。** 1 度きりの実行を空の runner 一覧で
// 使い切ってしまい、runner ごとに判定する 2 項目（NOPASSWD sudo / docker グループ
// 所属）がセッション中一度も走らず、警告も出ない（discover.go の applyDiscovered）。
//
// **再検出のたびにも走らせない。** 判定対象はホストの構成であって秒単位で
// 変わらず、`sudo -l -U` は監査ログに記録されるため他のレコードを押し流す。
func TestStartupHostRequirementRunsOnceAfterFirstSuccess(t *testing.T) {
	a := withHostChecks(newApp(exec.NewFake()), pagetest.StubCheck{Status: check.Fail})

	a, cmd := discovered(a, errTest)
	if _, err := takeHostReq(a, cmd); !errors.Is(err, pagetest.ErrNotFound) {
		t.Fatalf("検出に失敗した周期で前提チェックが発行された（空の runner 一覧で使い切る）: err = %v", err)
	}

	a, cmd = discovered(a, nil)
	a, err := takeHostReq(a, cmd)
	if err != nil {
		t.Fatalf("検出に成功した周期でも前提チェックが発行されない（FR-44 が走らない）: %v", err)
	}
	if a.bg.HostReq.Bad() != 1 {
		t.Errorf("届いた件数 = %d, want 1", a.bg.HostReq.Bad())
	}

	for range 3 {
		var next tea.Cmd
		a, next = discovered(a, nil)
		if _, err := takeHostReq(a, next); !errors.Is(err, pagetest.ErrNotFound) {
			t.Fatalf("再検出のたびに前提チェックが走っている: err = %v", err)
		}
	}
}
