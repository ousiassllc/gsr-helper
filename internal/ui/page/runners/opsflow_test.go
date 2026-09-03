package runners_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/exec"
	"github.com/ousiassllc/gsr-helper/internal/runner"
	"github.com/ousiassllc/gsr-helper/internal/svc"
	"github.com/ousiassllc/gsr-helper/internal/ui/atom"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/pagetest"
)

// 一括操作（FR-08）・可否の再判定（FR-09）・詳細画面からの起動（FR-46）の検証を集める。

// hintFor はフッタから 1 つのキーヒントを取り出す。
func hintFor(t *testing.T, c page.ChromeMsg, k string) atom.Hint {
	t.Helper()

	for _, h := range c.Footer {
		if h.Key == k {
			return h
		}
	}
	t.Fatalf("フッタに %q のヒントが無い（%+v）", k, c.Footer)
	return atom.Hint{Key: "", Desc: "", Enabled: false, Reason: ""}
}

// 選択した runner 全件に同じ操作が適用される（FR-08）。
//
// **1 件目が失敗しても残りを実行する。** メンテナンス前の全停止で 1 台の失敗が
// 残りを止めると、止まった台と止まらなかった台が混在したまま作業に入ることになる。
func TestBulkOperationAppliesToEverySelectedRunner(t *testing.T) {
	st, f := opsState()
	// 1 台目の停止だけを失敗させる。2 台目まで到達することを見るためである。
	f.SetFunc(func(_ string, args []string) (exec.Result, error) {
		if len(args) > 1 && args[len(args)-1] == unit1 {
			return exec.Result{Stdout: nil, Stderr: []byte("Unit not loaded."), ExitCode: 5}, nil
		}
		return exec.Result{Stdout: nil, Stderr: nil, ExitCode: 0}, nil
	})
	m, _ := newModel(t, st)

	// space で 1 台目、j + space で 2 台目を選ぶ。
	m = opsSend(t, m, "space", "j", "space")
	if got := opsChrome(t, m).Status; !strings.Contains(got, "選択: 2 件") {
		t.Fatalf("状態行 = %q, want 選択: 2 件", got)
	}

	m = opsSend(t, m, "x", "y")

	want := []string{"systemctl stop " + unit1, "systemctl stop " + unit2}
	if got := issued(f); !reflect.DeepEqual(got, want) {
		t.Fatalf("発行コマンド = %v, want %v（1 件失敗で打ち切っていないか）", got, want)
	}

	status := opsChrome(t, m).Status
	for _, w := range []string{"停止", "1 件成功", "1 件失敗", "build01-1"} {
		if !strings.Contains(status, w) {
			t.Errorf("状態行 = %q, %q を含んでいない", status, w)
		}
	}
	// 実行し終えた集合は選択を解く（同じ集合へ二度目の一括操作を打てないように）。
	if strings.Contains(status, "選択:") {
		t.Errorf("状態行 = %q, 実行後も選択が残っている", status)
	}
}

// 一括操作の確認ダイアログは対象全件とコマンド全件を出す。
//
// 「N 台に適用します」だけでは、どの runner にどのコマンドが走るのかを承認できない
// （screens.md の確認フローが求めるのは対象と実行コマンド**全文**である）。
func TestBulkConfirmListsEveryTargetAndCommand(t *testing.T) {
	st, _ := opsState()
	m, _ := newModel(t, st)

	m = opsSend(t, m, "space", "j", "space", "x")
	body := m.View().Content
	for _, want := range []string{"build01-1", "build01-2", "systemctl stop " + unit1, "systemctl stop " + unit2} {
		if !strings.Contains(body, want) {
			t.Errorf("確認ダイアログに %q が無い:\n%s", want, body)
		}
	}
}

// 塞がれた操作はフッタに理由を出し、キーを打っても何も起こさない（FR-09、
// screens.md の無効な操作の表示）。
//
// **理由と実行経路の両方を見る。** 理由だけを見ると「グレーアウトしているのに打てば
// 動く」退行を、コマンドだけを見ると「動かないが理由も出ない」退行を見逃す。
//
// 塞ぐ理由（直起動・非 root）が変わっても**固定する内容は同じ**なので表でまとめる。
func TestBlockedOperationsAreHintedAndIssueNothing(t *testing.T) {
	// hinted はフッタで理由を確かめるキー、keys は実際に打つキー。R はフッタに
	// 載らない（幅の都合。keymap.RunnerKeys.Footer）が、キーとしては効く。
	tests := map[string]struct {
		runners []runner.Runner
		root    bool
		hinted  []string
		keys    []string
		reason  string
	}{
		"run.sh 直起動では s / x / R が塞がれる": {
			[]runner.Runner{pagetest.StandaloneRunner()}, true,
			[]string{"s", "x"}, []string{"s", "x", "R"}, svc.ReasonStandalone,
		},
		"非 root では s / x / X / R が塞がれる": {
			nil, false,
			[]string{"s", "x", "X"}, []string{"s", "x", "X", "R"}, svc.ReasonRoot,
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			st, f := opsState(tt.runners...)
			st.Caps.Root = tt.root
			m, _ := newModel(t, st)

			c := opsChrome(t, m)
			for _, k := range tt.hinted {
				h := hintFor(t, c, k)
				if h.Enabled {
					t.Errorf("%q が有効になっている", k)
				}
				if h.Reason != tt.reason {
					t.Errorf("%q の理由 = %q, want %q", k, h.Reason, tt.reason)
				}
			}

			m = opsSend(t, m, tt.keys...)
			if got := issued(f); len(got) != 0 {
				t.Errorf("塞がれているのにコマンドが発行された: %v", got)
			}
			if got := opsChrome(t, m); got.Modal {
				t.Error("塞がれているのに確認ダイアログが開いた")
			}
			if got := opsChrome(t, m).Status; !strings.Contains(got, tt.reason) {
				t.Errorf("状態行 = %q, 理由 %q を出していない", got, tt.reason)
			}
		})
	}
}

// 詳細画面から選んだ操作は、**その詳細画面が開いている runner 1 件だけ**に効く。
//
// 一覧で他の runner を選択済みでも巻き込まない。詳細画面は「その runner に対する
// 操作」の起点であり、画面に出ている対象と実行される対象が食い違うと、見て承認した
// ことにならない（一覧の直接キーが一括選択を見るのと意図的に違う）。
//
// **確認の強さは起点で変わらない**（FR-45〜FR-47）。破壊的操作なら詳細画面から
// 起動しても同じ確認ダイアログを経る。
func TestDetailOperationTargetsOnlyItsRunner(t *testing.T) {
	st, f := opsState()
	m, _ := newModel(t, st)

	// 1 台目と 2 台目を選択したうえで、2 台目の詳細を開く。
	m = opsSend(t, m, "space", "j", "space", "enter")
	if !opsChrome(t, m).Modal {
		t.Fatal("詳細画面が開いていない")
	}

	// 詳細画面の直接キーで停止を選ぶ。確認ダイアログが重なる（詳細は閉じない）。
	m = opsSend(t, m, "x")
	if body := m.View().Content; !strings.Contains(body, "停止の確認") {
		t.Fatalf("確認ダイアログが重なっていない:\n%s", body)
	}
	if got := issued(f); len(got) != 0 {
		t.Fatalf("確認の前にコマンドが発行されている: %v", got)
	}

	m = opsSend(t, m, "y")
	want := []string{"systemctl stop " + unit2}
	if got := issued(f); !reflect.DeepEqual(got, want) {
		t.Errorf("発行コマンド = %v, want %v（一覧の選択を巻き込んでいないか）", got, want)
	}
	// 確認を 1 枚閉じた後も詳細画面は残る（重ねただけである）。
	if !opsChrome(t, m).Modal {
		t.Error("確認を閉じたときに詳細画面まで閉じている")
	}
}
