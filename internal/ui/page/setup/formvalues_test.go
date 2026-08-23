// 入力の受け皿（formValues）と検証関数を直に確かめる。外の setup_test からは
// 触れないため、このファイルだけ内部テストにしてある。
package setup

import (
	"errors"
	"strings"
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/appconfig"
	"github.com/ousiassllc/gsr-helper/internal/setup"
	"github.com/ousiassllc/gsr-helper/internal/setup/valid"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/pagetest"
)

// testState は既定だけを載せた共有状態を返す。外部資源は差し替えておく
// （外の helper_test.go の stateAPI と同じ理由。page.SetupDeps の doc）。
func testState(t *testing.T) page.StateMsg {
	st := pagetest.State(100, 30)
	st.Setup = pagetest.NewSetupAPI(t.Cleanup).Deps("build01", appconfig.Default().Defaults)
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
