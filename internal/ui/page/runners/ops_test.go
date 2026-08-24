package runners_test

import (
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/exec"
	"github.com/ousiassllc/gsr-helper/internal/runner"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/pagetest"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/pagetest/cmdtest"
)

// サービス制御（Issue #5）の検証を集める。**発行された systemctl のコマンド列**を
// exec.Fake の記録で見る。画面の文字列ではなくコマンドで見るのは、確認を経ずに
// 実行される退行がここでしか捕まらないためである。

// unit1 / unit2 は検証に使う systemd ユニット名（sampleRunner が付ける名前）。
const (
	unit1 = "actions.runner.foo.build01-1.service"
	unit2 = "actions.runner.foo.build01-2.service"
)

// opsState は Fake の Executor を持つ共有状態と、その Fake を返す。
//
// pagetest.State も Fake を持たせるが手元に返してくれないため、ここで差し替える。
func opsState(rs ...runner.Runner) (page.StateMsg, *exec.Fake) {
	if len(rs) == 0 {
		rs = []runner.Runner{sampleRunner("build01-1", false), sampleRunner("build01-2", false)}
	}
	st := pagetest.State(80, 20, rs...)
	f := exec.NewFake()
	st.Exec = f
	return st, f
}

// opsSend はキーを順に送り、そのつど非同期の往復（確認 → 実行 → 結果）を回す。
func opsSend(t *testing.T, m tea.Model, keys ...string) tea.Model {
	t.Helper()

	for _, k := range keys {
		next, cmd := m.Update(press(k))
		m = cmdtest.Advance(next, cmd, cmdtest.AdvanceRounds)
	}
	return m
}

// opsChrome は状態を変えない Msg を 1 つ送って、今の ChromeMsg を取り出す。
//
// 打鍵の戻り値から取ると、非同期の結果が届く**前**の状態を見ることになる。
func opsChrome(t *testing.T, m tea.Model) page.ChromeMsg {
	t.Helper()

	_, cmd := m.Update(tea.WindowSizeMsg{Width: 80, Height: 20})
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

// 一覧の直接キーが、対象 runner に対する期待どおりのコマンドを発行する。
//
// **破壊的操作（x / X / R）は y を打つまで 1 本も発行しない。** これが「確認を
// 省略する近道を作らない」（FR-45〜FR-47）の実体であり、確認ダイアログを開くだけで
// 実行してしまう退行はここで落ちる。
func TestRunnerKeysIssueExpectedCommands(t *testing.T) {
	tests := map[string]struct {
		keys  []string
		want  []string
		modal bool // y を打つ前に確認ダイアログが開くか
	}{
		"開始は確認なしで実行":          {[]string{"s"}, []string{"systemctl start " + unit1}, false},
		"切替は確認なしで disable":    {[]string{"E"}, []string{"systemctl disable " + unit1}, false},
		"停止は y のあとに実行":        {[]string{"x", "y"}, []string{"systemctl stop " + unit1}, true},
		"再起動は y のあとに実行":       {[]string{"R", "y"}, []string{"systemctl restart " + unit1}, true},
		"強制停止は y のあとに 2 段で実行": {[]string{"X", "y"}, []string{"kill -KILL 100", "systemctl stop " + unit1}, true},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			st, f := opsState()
			m, _ := newModel(t, st)

			m = opsSend(t, m, tt.keys[0])
			if got := opsChrome(t, m).Modal; got != tt.modal {
				t.Fatalf("1 打鍵目のあとのモーダル = %v, want %v", got, tt.modal)
			}
			if tt.modal && len(f.Calls()) != 0 {
				t.Fatalf("確認の前にコマンドが発行されている: %v", issued(f))
			}

			m = opsSend(t, m, tt.keys[1:]...)
			if got := issued(f); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("発行コマンド = %v, want %v", got, tt.want)
			}
			if opsChrome(t, m).Modal {
				t.Error("実行後も確認ダイアログが開いている")
			}
		})
	}
}

// 発行するコマンドが 1 本も無い runner では、確認ダイアログを開かずに理由を出す。
//
// 未稼働でサービス未インストールの runner（runner.ManagedUnknown。ユニット名も PID も
// 無い）は、可否の判定（svc.CanControl）では X を塞がれない。しかし強制停止が発行
// できるコマンドは 0 本で、確認ダイアログは「実行するコマンド:」の見出しごと消えた
// まま開く。**コマンドが 1 行も出ないダイアログで y を押させ、そのうえで失敗させる**
// のは承認の意味を失わせる（実行側も svc.ErrNoKillTarget で失敗する）。押す前に
// 「実行できる対象がありません」と伝える。
func TestKillWithoutAnyTargetSkipsConfirm(t *testing.T) {
	st, f := opsState(unmanagedRunner("build01-1"))
	m, _ := newModel(t, st)

	m = opsSend(t, m, "X")

	c := opsChrome(t, m)
	if c.Modal {
		t.Error("実行できる対象が無いのに確認ダイアログが開いている")
	}
	if got := issued(f); len(got) != 0 {
		t.Errorf("コマンドが発行された: %v", got)
	}
	if !strings.Contains(c.Status, "実行できる対象がありません") {
		t.Errorf("状態行 = %q, want 実行できる対象がありません を含む", c.Status)
	}
}

// 確認をキャンセルするキー（n / esc / enter）では 1 本も発行しない。
//
// **enter を含めるのが要点である。** 一覧の enter で詳細を開き詳細の enter で操作を
// 選ぶ流れの勢いのまま確認の enter を押しても、破壊的操作が走ってはならない
// （screens.md の確認ダイアログ。既定はキャンセル）。
func TestConfirmCancelIssuesNothing(t *testing.T) {
	for _, k := range []string{"n", "esc", "enter"} {
		t.Run(k, func(t *testing.T) {
			st, f := opsState()
			m, _ := newModel(t, st)

			m = opsSend(t, m, "x", k)
			if got := issued(f); len(got) != 0 {
				t.Errorf("キャンセルなのにコマンドが発行された: %v", got)
			}
			if opsChrome(t, m).Modal {
				t.Error("キャンセルで確認ダイアログが閉じていない")
			}
		})
	}
}

// 確認ダイアログには対象・影響・実行コマンド全文が出る。
//
// **実行コマンド全文の表示は確認フローの必須要素である**（screens.md の確認フロー）。
// ジョブ実行中の runner が混じるときは警告とドレイン停止の案内も出す
// （functional.md の確認フロー図の「ジョブ実行中? → 警告・ドレインを促す」）。
func TestConfirmDialogShowsTargetsAndCommand(t *testing.T) {
	st, _ := opsState(sampleRunner("build01-1", true))
	m, _ := newModel(t, st)

	m = opsSend(t, m, "X")
	body := m.View().Content
	for _, want := range []string{
		"build01-1",
		"実行中のジョブは中断されます",
		"ジョブ実行中: build01-1",
		"ドレイン停止",
		"kill -KILL 100",
		"systemctl stop " + unit1,
		"実行しますか? [y/N]",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("確認ダイアログに %q が無い:\n%s", want, body)
		}
	}
}

// モーダル表示中はグローバルキーを親へ差し戻さない（screens.md のモーダル表示中）。
//
// 差し戻すと、確認中に打った q でアプリが終わり、1 でタブが変わる。確認ダイアログは
// **破壊的操作の直前**に開くモーダルなので、ここで閉じ込めが破れると影響が最も大きい。
func TestConfirmModalSwallowsGlobalKeys(t *testing.T) {
	st, _ := opsState()
	m, _ := newModel(t, st)
	m = opsSend(t, m, "x")

	for _, k := range []string{"q", "1", "r", "?"} {
		_, cmd := m.Update(press(k))
		c, global, bubbled := pagetest.ScanKey(cmd)
		if bubbled {
			t.Errorf("確認中の %q が親へ差し戻された（%v）", k, global.Press)
		}
		if !c.Modal {
			t.Errorf("確認中の %q でモーダルが閉じた", k)
		}
	}
}
