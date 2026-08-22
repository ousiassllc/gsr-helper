package runnerop

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/runner"
	"github.com/ousiassllc/gsr-helper/internal/runner/systemd"
	"github.com/ousiassllc/gsr-helper/internal/svc"
	"github.com/ousiassllc/gsr-helper/internal/ui/keymap"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/action"
)

// 制御部の部品（対象の選び方・結果の文・確認の中身）を単体で固定する。
//
// 経路そのもの（キー → 確認 → 実行 → 報告）はタブ側のテスト（page/runners・page/jobs）が
// 端から端まで通す。ここはその経路に載る値の作り方だけを見る。

// testRunner は systemd 管理の runner を返す。
func testRunner(name string) runner.Runner {
	dir := "/opt/runners/" + name
	unit := "actions.runner.foo." + name + ".service"
	return runner.Runner{
		Dir:      dir,
		Config:   runner.Config{AgentName: name},
		UnitName: unit,
		Managed:  runner.ManagedSystemd,
		Svc:      &systemd.State{Unit: unit, Load: "loaded", Active: "active", Sub: "running", FileState: "enabled"},
	}
}

// busy はジョブ実行中の runner を返す。
func busy(name string) runner.Runner {
	r := testRunner(name)
	r.Workers = []runner.Process{{PID: 284193, Kind: runner.ProcWorker, Dir: r.Dir}}
	return r
}

// 対象の選び方は「選択があればそれ全部、無ければカーソル位置の 1 件」である（FR-08）。
func TestTargets(t *testing.T) {
	a, b, cur := testRunner("build01-1"), testRunner("build01-2"), testRunner("build01-9")

	tests := map[string]struct {
		checked []runner.Runner
		ok      bool
		want    []string
	}{
		"選択があればそれ全部":       {[]runner.Runner{a, b}, true, []string{"build01-1", "build01-2"}},
		"選択が無ければカーソルの 1 件": {nil, true, []string{"build01-9"}},
		"カーソルも無ければ空":       {nil, false, nil},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			got := make([]string, 0, 2)
			for _, r := range Targets(tt.checked, cur, tt.ok) {
				got = append(got, r.Name())
			}
			if len(got) == 0 {
				got = nil
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Targets = %v, want %v", got, tt.want)
			}
		})
	}
}

// 一覧で直接受けるキーの集合は、この版で実装した操作だけである。
//
// 未実装の操作（n / D / u / e / l）を混ぜると「押せるが何も起きない」経路ができる。
// Jobs タブは screens.md の Jobs タブのフッタに合わせて 3 つに絞る（FR-47）。
func TestDirectKeySets(t *testing.T) {
	if got, want := ListOps(), []action.ID{
		action.Start, action.Stop, action.Kill, action.Drain, action.Restart, action.Enable,
	}; !reflect.DeepEqual(got, want) {
		t.Errorf("ListOps = %v, want %v", got, want)
	}
	if got, want := JobsOps(), []action.ID{action.Drain, action.Kill, action.Restart}; !reflect.DeepEqual(got, want) {
		t.Errorf("JobsOps = %v, want %v", got, want)
	}

	// 直接受ける操作はすべてキー定義に対応が要る（対応が無いと HandleKey が拾えない）。
	keys := keymap.NewRunnerKeys()
	for _, id := range append(ListOps(), JobsOps()...) {
		if _, ok := binding(id, keys); !ok {
			t.Errorf("操作 %s に対応するキー定義が無い", id)
		}
	}
}

// 確認ダイアログを経るのは x / X / R である（screens.md の Runners タブの操作）。
//
// d は待機の開始でありキャンセルできるため、s と E は元へ戻せるため確認を求めない。
func TestNeedsConfirm(t *testing.T) {
	want := map[action.ID]bool{
		action.Start: false, action.Stop: true, action.Kill: true,
		action.Drain: false, action.Restart: true, action.Enable: false,
	}
	for id, w := range want {
		if got := needsConfirm(id); got != w {
			t.Errorf("needsConfirm(%s) = %v, want %v", id, got, w)
		}
	}
}

// 確認の中身は対象・影響・実行コマンド全文で組む。
//
// コマンドは svc.CommandLine から引く。**UI 側で組み立て直すと、承認した内容と
// 実際に走るコマンドが食い違う**（svc/commandline.go の doc）。
func TestConfirmInput(t *testing.T) {
	set := action.NewSet(keymap.NewRunnerKeys())
	def := action.Def{}
	for _, d := range set.List() {
		if d.ID == action.Stop {
			def = d
		}
	}

	targets := []runner.Runner{testRunner("build01-1"), busy("build01-2")}
	in := confirmInput(def, targets)

	if in.Title != "停止の確認" {
		t.Errorf("見出し = %q, want 停止の確認", in.Title)
	}
	if want := []string{"build01-1", "build01-2"}; !reflect.DeepEqual(in.Targets, want) {
		t.Errorf("対象 = %v, want %v", in.Targets, want)
	}

	want := make([]string, 0, 2)
	for _, r := range targets {
		want = append(want, svc.CommandLine(svc.OpStop, r)...)
	}
	if !reflect.DeepEqual(in.Command, want) {
		t.Errorf("コマンド = %v, want %v（svc.CommandLine と食い違っている）", in.Command, want)
	}

	joined := strings.Join(in.Impact, "\n")
	if !strings.Contains(joined, "build01-2") || !strings.Contains(joined, "ドレイン停止") {
		t.Errorf("影響 = %v, ジョブ実行中の警告とドレインの案内が無い", in.Impact)
	}
	// ジョブ実行中が 1 台も無ければ警告は出さない（毎回出すと読み飛ばされる）。
	// 操作そのものの影響（action.Def.Impact）は対象に依らず出るので、残るのはその
	// 1 行だけである（confirmimpact_test.go）。
	idle := confirmInput(def, []runner.Runner{testRunner("build01-1")})
	if want := []string{def.Impact}; !reflect.DeepEqual(idle.Impact, want) {
		t.Errorf("影響 = %v, want %v（ジョブ実行中の警告が出ている）", idle.Impact, want)
	}
}

// 結果の報告は成功件数だけでなく、失敗した runner と理由まで出す。
func TestDescribe(t *testing.T) {
	fail := errors.New("コマンドが失敗しました（終了コード 5）")

	tests := map[string]struct {
		results []Result
		note    note
		want    string
	}{
		"全件成功": {
			[]Result{{Runner: "a", Err: nil}, {Runner: "b", Err: nil}},
			note{skipped: 0, canceled: false},
			"停止: 2 件成功",
		},
		"一部失敗": {
			[]Result{{Runner: "a", Err: nil}, {Runner: "b", Err: fail}},
			note{skipped: 0, canceled: false},
			"停止: 1 件成功 1 件失敗（b: コマンドが失敗しました（終了コード 5））",
		},
		"除外あり": {
			[]Result{{Runner: "a", Err: nil}},
			note{skipped: 2, canceled: false},
			"停止: 1 件成功 2 件は実行できないため除外",
		},
		"何もせずキャンセル": {
			nil,
			note{skipped: 0, canceled: true},
			"停止: キャンセルしました",
		},
		"1 件終えてからキャンセル": {
			[]Result{{Runner: "a", Err: nil}},
			note{skipped: 0, canceled: true},
			"停止: キャンセルしました 1 件成功",
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if got := describe("停止", tt.results, tt.note); got != tt.want {
				t.Errorf("describe = %q, want %q", got, tt.want)
			}
		})
	}
}

// 複数行のエラーは 1 行目だけを状態行へ出す。
//
// 強制停止は kill と systemctl stop の失敗を errors.Join でまとめる（svc.Kill）ため
// 改行を含みうる。そのまま出すと状態行の枠が崩れる。
func TestFirstLine(t *testing.T) {
	if got, want := firstLine("1 行目\n2 行目"), "1 行目 …"; got != want {
		t.Errorf("firstLine = %q, want %q", got, want)
	}
	if got, want := firstLine("1 行だけ"), "1 行だけ"; got != want {
		t.Errorf("firstLine = %q, want %q", got, want)
	}
}

// 対象が 1 件も残らなかったときは理由を添える（押しても何も起きないように見せない）。
func TestNoTargetText(t *testing.T) {
	if got, want := noTargetText(0, ""), "対象がありません"; got != want {
		t.Errorf("noTargetText = %q, want %q", got, want)
	}
	got := noTargetText(2, svc.ReasonStandalone)
	if !strings.Contains(got, svc.ReasonStandalone) {
		t.Errorf("noTargetText = %q, 理由を含んでいない", got)
	}
}
