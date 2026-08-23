package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/doctor/check"
	"github.com/ousiassllc/gsr-helper/internal/exec"
	"github.com/ousiassllc/gsr-helper/internal/runner"
	"github.com/ousiassllc/gsr-helper/internal/ui/chrome"
	"github.com/ousiassllc/gsr-helper/internal/ui/hostreq"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/pagetest"
)

// 発行そのもの（件数の数え方・実行回数）は internal/ui/hostreq が検証する。
// ここで見るのは結果がヘッダと状態行まで届くことだけである。

// discovered は検出が 1 周期終わった App と、そのとき返った Cmd を返す。
func discovered(a App) (App, tea.Cmd) {
	return update(a, discoveredMsg{
		result: runner.Result{Runners: []runner.Runner{sampleRunner()}},
		err:    nil,
	})
}

// takeHostReq は Cmd の束から前提チェックの結果を取り込む。無ければ ok が偽。
func takeHostReq(a App, cmd tea.Cmd) (App, bool) {
	for _, c := range pagetest.Expand(cmd) {
		if c == nil {
			continue
		}
		if msg, ok := c().(hostreq.Msg); ok {
			a, _ = update(a, msg)
			return a, true
		}
	}
	return a, false
}

// 起動時の前提チェック（FR-44）の結果がヘッダと状態行に出て、Doctor タブへ誘導される。
func TestStartupHostRequirementReachesChrome(t *testing.T) {
	tests := map[string]struct {
		status check.Status
		warn   bool
	}{
		"不備があれば警告する":       {status: check.Fail, warn: true},
		"注意も警告する":          {status: check.Warn, warn: true},
		"正常なら警告しない":        {status: check.OK, warn: false},
		"能力不足（SKIP）は警告しない": {status: check.Skip, warn: false},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			a := withHostChecks(newApp(exec.NewFake()), pagetest.StubCheck{Status: tt.status})
			a, _ = update(a, tea.WindowSizeMsg{Width: 100, Height: 24})

			a, cmd := discovered(a)
			a, ok := takeHostReq(a, cmd)
			if !ok {
				t.Fatal("起動時の前提チェックが発行されていない（FR-44）")
			}

			status, header := chrome.Status(a.chromeView()), chrome.Header(a.chromeView())
			if !tt.warn {
				for label, line := range map[string]string{"状態行": status, "ヘッダ": header} {
					if strings.Contains(line, "ホスト前提") {
						t.Errorf("%sに警告が出ている: %q", label, line)
					}
				}
				return
			}

			if !strings.Contains(status, "ホスト前提 1 件") {
				t.Errorf("状態行に警告が無い: %q", status)
			}
			// 誘導は Doctor タブの番号キーで示す（screens.md の共通レイアウト）。
			if !strings.Contains(status, "（5 で詳細）") {
				t.Errorf("状態行に Doctor タブへの誘導が無い: %q", status)
			}
			if !strings.Contains(header, "ホスト前提 1 件") {
				t.Errorf("ヘッダに警告が無い: %q", header)
			}
		})
	}
}

// **再検出のたびには走らせない。** 判定対象はホストの構成であって秒単位で
// 変わらず、`sudo -l -U` は監査ログに記録されるため他のレコードを押し流す。
func TestStartupHostRequirementRunsOnce(t *testing.T) {
	a := withHostChecks(newApp(exec.NewFake()), pagetest.StubCheck{Status: check.Fail})

	a, cmd := discovered(a)
	a, ok := takeHostReq(a, cmd)
	if !ok {
		t.Fatal("起動時の前提チェックが発行されていない")
	}

	for range 3 {
		var next tea.Cmd
		a, next = discovered(a)
		if _, again := takeHostReq(a, next); again {
			t.Fatal("再検出のたびに前提チェックが走っている")
		}
	}
}
