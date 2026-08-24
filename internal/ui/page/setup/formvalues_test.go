// 入力の受け皿（formValues）・検証関数・結果報告の組み立てを直に確かめる。
// いずれも外の setup_test からは触れないため、このファイルだけ内部テストにしてある。
package setup

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/setup"
	"github.com/ousiassllc/gsr-helper/internal/setup/valid"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/pagetest"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/pagetest/cmdtest"
)

// testState は既定だけを載せた共有状態を返す。外部資源は差し替え済み
// （pagetest.SetupState の doc）。
func testState(t *testing.T) page.StateMsg {
	st, _ := pagetest.SetupState(t.Cleanup)
	return st
}

// 中断したフォームの入力を次のフォームへ持ち越さない。runner group の入力欄が
// あるのは 1 台ずつのフォームだけなのに spec は種類を問わず RunnerGroup を送る
// ため、消し忘れると一括追加の画面に出ていない --runnergroup が全台に付く。
func TestResetClearsValuesTheNextFormDoesNotShow(t *testing.T) {
	t.Parallel()

	st := testState(t)
	v := newValues(st)

	// 1 台ずつのフォームで runner group まで入れ、そのまま中断する。
	v.reset(st, wizardForm)
	v.url, v.name, v.runnerGroup = "https://github.com/orgs/foo", "gpu-box", "gpu"

	v.reset(st, bulkForm) // 次に一括追加を開く

	if v.runnerGroup != "" {
		t.Errorf("runnerGroup = %q, want 空（一括追加の画面に入力欄が無い）", v.runnerGroup)
	}
	if v.url != "" {
		t.Errorf("url = %q, want 空（中断した入力を持ち越さない）", v.url)
	}

	v.url = "https://github.com/orgs/foo"
	spec, err := v.spec(st)
	if err != nil {
		t.Fatalf("spec: %v", err)
	}
	if spec.RunnerGroup != "" {
		t.Errorf("AddSpec.RunnerGroup = %q, want 空", spec.RunnerGroup)
	}
	plan, err := setup.PlanAdd(spec)
	if err != nil {
		t.Fatalf("PlanAdd: %v", err)
	}
	for _, u := range plan.Units {
		if cmds := strings.Join(u.CommandLines(), "\n"); strings.Contains(cmds, "--runnergroup") {
			t.Errorf("コマンドに --runnergroup が残っている:\n%s", cmds)
		}
	}
}

// 検証関数はドメイン層（internal/setup/valid）へ委ね、UI で規則を持たない。
// 委ね先を取り違えていないことだけを確かめる。
func TestValidators(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		fn   func(string) error
		in   string
		want error
	}{
		"URL":             {validateURL, "https://github.com/orgs/foo", nil},
		"URL が GitHub 外":  {validateURL, "https://example.test/foo", valid.ErrNotGitHub},
		"インストール先":         {validateBase, "/opt/runners", nil},
		"インストール先が相対":      {validateBase, "runners", valid.ErrNotAbs},
		"runner 名":        {validateName, "build01-1", nil},
		"runner 名が空":      {validateName, "", valid.ErrEmptyName},
		"work dir":        {validateWork, "_work", nil},
		"work dir が空":     {validateWork, "", nil},
		"work dir の先頭が -": {validateWork, "-rf", valid.ErrLeadingDash},
		"ラベル":             {validateLabels, "gpu,cuda", nil},
		"ラベルが予約語":         {validateLabels, "linux", valid.ErrReservedLabel},
		"任意項目の先頭が -":      {validateOptional, "-rf", valid.ErrLeadingDash},
		"台数":              {validateCount, "3", nil},
		"台数が数値でない":        {validateCount, "three", valid.ErrBadCount},
		"台数が範囲外":          {validateCount, "51", valid.ErrBadCount},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if err := tt.fn(tt.in); !errors.Is(err, tt.want) {
				t.Errorf("%q の検証 = %v, want %v", tt.in, err, tt.want)
			}
		})
	}
}

// spec は画面に出ている入力を余さず AddSpec へ渡す（AC-2）。
//
// **既定と同じ値で埋めた検証では足りない。** work dir を既定（_work）のまま送る
// 筋書きしか無いと、入力欄を読まずに既定を書き込む実装でも通る。ラベル・
// runner group・ephemeral も同じ理由で既定と違う値にして送る。
func TestSpecCarriesEveryWizardInput(t *testing.T) {
	t.Parallel()

	st := testState(t)
	v := newValues(st)
	v.reset(st, wizardForm)
	v.url, v.name = "https://github.com/orgs/foo", "gpu-box"
	v.workDir, v.runnerGroup, v.labels, v.ephemeral = "scratch", "gpu", "gpu,cuda", true

	spec, err := v.spec(st)
	if err != nil {
		t.Fatalf("spec: %v", err)
	}

	got := []any{spec.Names, spec.WorkDir, spec.RunnerGroup, spec.Labels, spec.Ephemeral}
	want := []any{[]string{"gpu-box"}, "scratch", "gpu", []string{"gpu", "cuda"}, true}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("AddSpec の入力由来の項目 =\n %v\nwant\n %v", got, want)
	}
}

// reportLines は AC-5 の結果報告そのものである。
//
// 失敗したときは「どこで止まったか」と「成功分がそのまま残っていること」を必ず出す。
// **残る旨が消えると、作りかけの runner を手で消すべきか判断できない**（FR-15）。
func TestReportLines(t *testing.T) {
	t.Parallel()

	if got := reportLines(setup.Result{Succeeded: []string{"a", "b"}, Failed: "",
		Phase: "", Err: nil, Remaining: nil}, nil); len(got) != 1 || got[0] != "完了: 2 台" {
		t.Errorf("成功時の報告 = %q, want [完了: 2 台]", got)
	}

	res := setup.Result{
		Succeeded: []string{"build01-1", "build01-2"}, Failed: "build01-3",
		Phase: "サービス登録", Err: errors.New("boom"), Remaining: []string{"build01-4"},
	}
	got := strings.Join(reportLines(res, res.Err), "\n")
	for _, w := range []string{
		"✗ build01-3 のサービス登録で失敗しました", "  boom",
		"完了: 2 台（build01-1, build01-2）", "未実行: 1 台（build01-4）",
		"build01-1, build01-2 はそのまま残っています。",
	} {
		if !strings.Contains(got, w) {
			t.Errorf("結果報告に %q が無い:\n%s", w, got)
		}
	}
}

// 登録の Cmd は最初の共有状態で 1 度だけ親へ流れる（flushInit）。
//
// 印を差し込んで数えるのは、いま登録が返す Cmd がどれも nil で、流れたかを外から
// 見分けられないためである（Msg を見る形では「1 度も流れない」実装まで緑になる）。
func TestRegisterCmdReachesParentOnlyOnce(t *testing.T) {
	t.Parallel()

	st := testState(t)
	m := New(0, st)
	if cmd := m.Init(); cmd != nil {
		t.Error("Init が Cmd を返している（親はタブの Init を呼ばない）")
	}

	flowed := 0
	m.initCmd = func() tea.Msg { flowed++; return nil }

	next, first := m.Update(st)
	cmdtest.RunAll(first)
	if flowed != 1 {
		t.Fatalf("最初の共有状態で登録の Cmd が流れた回数 = %d, want 1", flowed)
	}

	_, second := next.Update(st)
	cmdtest.RunAll(second)
	if flowed != 1 {
		t.Errorf("登録の Cmd が流れた累計 = %d, want 1（2 度目にも流れている）", flowed)
	}
}
