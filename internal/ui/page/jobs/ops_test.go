package jobs_test

import (
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/exec"
	"github.com/ousiassllc/gsr-helper/internal/runner"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/jobs"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/pagetest"
)

// Jobs タブからのサービス制御（FR-47）の検証を集める。
//
// **Runners タブと同じ経路（page/runnerop）を通ることを、同じ観点で確かめる。**
// 起点が違っても確認の強さが変わらないことは規約では守れず、片方のタブだけ確認を
// 飛ばす退行はコンパイルも既存の検査も通る。

// jobsUnit は検証に使う systemd ユニット名（busyRunner が付ける名前）。
const jobsUnit = "actions.runner.foo.build01-1.service"

// opsModel は Fake の Executor を持つ Jobs タブと、その Fake を返す。
func opsModel(t *testing.T, rs ...runner.Runner) (tea.Model, *exec.Fake) {
	t.Helper()

	st := pagetest.State(80, 16, rs...)
	f := exec.NewFake()
	st.Exec = f

	m, _ := jobs.New(1, st).Update(st)
	return m, f
}

// opsSend はキーを順に送り、そのつど非同期の往復（確認 → 実行 → 結果）を回す。
func opsSend(t *testing.T, m tea.Model, keys ...string) tea.Model {
	t.Helper()

	for _, k := range keys {
		next, cmd := m.Update(press(k))
		m = pagetest.Advance(next, cmd, pagetest.AdvanceRounds)
	}
	return m
}

// opsChrome は状態を変えない Msg を 1 つ送って、今の ChromeMsg を取り出す。
func opsChrome(t *testing.T, m tea.Model) page.ChromeMsg {
	t.Helper()

	_, cmd := m.Update(tea.WindowSizeMsg{Width: 80, Height: 16})
	return chrome(t, cmd)
}

// issued は Fake に記録されたコマンド行を返す。
func issued(f *exec.Fake) []string {
	out := make([]string, 0, len(f.Calls()))
	for _, c := range f.Calls() {
		out = append(out, c.String())
	}
	return out
}

// Jobs タブの直接キー（X / R）は Runners タブと同じ確認フローを経る（FR-47）。
//
// 操作対象はジョブではなく、そのジョブを実行している runner である。
func TestJobsKeysGoThroughTheSameConfirmFlow(t *testing.T) {
	tests := map[string]struct {
		keys []string
		want []string
	}{
		"強制停止": {[]string{"X", "y"}, []string{"kill -KILL 100 284000", "systemctl stop " + jobsUnit}},
		"再起動":  {[]string{"R", "y"}, []string{"systemctl restart " + jobsUnit}},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			m, f := opsModel(t, busyRunner("build01-1", 1))

			m = opsSend(t, m, tt.keys[0])
			if !opsChrome(t, m).Modal {
				t.Fatal("確認ダイアログが開いていない")
			}
			if got := issued(f); len(got) != 0 {
				t.Fatalf("確認の前にコマンドが発行されている: %v", got)
			}
			if body := m.View().Content; !strings.Contains(body, "実行しますか? [y/N]") {
				t.Fatalf("Runners タブと同じ確認ダイアログではない:\n%s", body)
			}

			opsSend(t, m, tt.keys[1:]...)
			if got := issued(f); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("発行コマンド = %v, want %v", got, tt.want)
			}
		})
	}
}

// 確認をキャンセルすれば 1 本も発行しない。
func TestJobsConfirmCancelIssuesNothing(t *testing.T) {
	m, f := opsModel(t, busyRunner("build01-1", 1))

	m = opsSend(t, m, "R", "n")
	if got := issued(f); len(got) != 0 {
		t.Errorf("キャンセルなのにコマンドが発行された: %v", got)
	}
	if opsChrome(t, m).Modal {
		t.Error("キャンセルで確認ダイアログが閉じていない")
	}
}

// ドレイン停止（d）は確認を経ずに待機画面を開く（Runners タブと同じ規則）。
func TestJobsDrainOpensWaiterWithoutConfirm(t *testing.T) {
	m, f := opsModel(t, busyRunner("build01-1", 1))

	// 待機の Cmd は流さない（流すと待機がその場で終わる）。
	next, _ := m.Update(press("d"))

	body := next.View().Content
	if strings.Contains(body, "実行しますか? [y/N]") {
		t.Fatalf("ドレイン停止で確認ダイアログが開いている:\n%s", body)
	}
	for _, want := range []string{"ドレイン停止中", "build01-1", "待機中も新しいジョブを受け付ける可能性があります"} {
		if !strings.Contains(body, want) {
			t.Errorf("待機画面に %q が無い:\n%s", want, body)
		}
	}
	if got := issued(f); len(got) != 0 {
		t.Errorf("待機を始めた時点でコマンドが発行された: %v", got)
	}
}

// 対象はカーソル位置のジョブを実行している runner 1 台である（FR-47）。
//
// Jobs タブは一括選択を持たない（一覧が選択不可）。同じ runner が 2 件のジョブを
// 実行していても、操作は runner 1 台に 1 回だけ効く。
func TestJobsOperationTargetsCursorRunner(t *testing.T) {
	m, f := opsModel(t, busyRunner("build01-1", 1), busyRunner("build01-7", 1))

	// カーソルを 2 行目（build01-7）へ動かしてから再起動する。
	opsSend(t, m, "j", "R", "y")

	want := []string{"systemctl restart actions.runner.foo.build01-7.service"}
	if got := issued(f); !reflect.DeepEqual(got, want) {
		t.Errorf("発行コマンド = %v, want %v", got, want)
	}
}

// この版で未対応の操作キー（l）は塞がれたままで、押しても何も起きない。
//
// フッタには出る（screens.md の設計原則 2「押しても何も起きないキーを出さない」の
// 対偶として、出すからには理由を添える）。ログは Logs タブを持ち込む Issue の担当。
func TestJobsUnsupportedKeyDoesNothing(t *testing.T) {
	m, f := opsModel(t, busyRunner("build01-1", 1))

	m = opsSend(t, m, "l")
	if got := issued(f); len(got) != 0 {
		t.Errorf("未対応のキーでコマンドが発行された: %v", got)
	}
	if opsChrome(t, m).Modal {
		t.Error("未対応のキーでモーダルが開いた")
	}
}
