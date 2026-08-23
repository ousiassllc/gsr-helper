package setup_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/setup"
	"github.com/ousiassllc/gsr-helper/internal/setup/valid"
)

func TestPlanAddNamesAndDirs(t *testing.T) {
	t.Parallel()

	p, err := setup.PlanAdd(addSpec())
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if p.Kind != setup.KindAdd {
		t.Errorf("Kind = %v, want KindAdd", p.Kind)
	}
	if want := []string{"build01-5", "build01-6", "build01-7"}; !slices.Equal(p.Names(), want) {
		t.Errorf("名前 = %v, want %v", p.Names(), want)
	}
	want := []string{"/opt/runners/build01-5", "/opt/runners/build01-6", "/opt/runners/build01-7"}
	if !slices.Equal(p.Dirs(), want) {
		t.Errorf("ディレクトリ = %v, want %v", p.Dirs(), want)
	}
	if !p.NeedsToken || !p.NeedsTarball {
		t.Errorf("NeedsToken/NeedsTarball = %v/%v, want true/true", p.NeedsToken, p.NeedsTarball)
	}
}

func TestPlanAddBuildsFullCommandLines(t *testing.T) {
	t.Parallel()

	p, err := setup.PlanAdd(addSpec())
	if err != nil {
		t.Fatalf("err = %v", err)
	}

	got := p.Units[0].CommandLines()
	want := []string{
		"./config.sh --url https://github.com/orgs/foo --token *** --name build01-5 " +
			"--labels gpu --work _work --runnergroup Default --unattended",
		"./svc.sh install",
		"./svc.sh start",
	}
	if !slices.Equal(got, want) {
		t.Errorf("コマンド全文:\n got: %v\nwant: %v", got, want)
	}
}

func TestPlanAddNeverCarriesTokenInPlan(t *testing.T) {
	t.Parallel()

	p, err := setup.PlanAdd(addSpec())
	if err != nil {
		t.Fatalf("err = %v", err)
	}

	for _, u := range p.Units {
		for _, s := range u.Steps {
			for i, a := range s.Args {
				if i == s.TokenIndex && a != "***" {
					t.Errorf("計画にトークンの位置が平文で入っている: %q", a)
				}
			}
		}
	}
}

func TestPlanAddOptionalFlags(t *testing.T) {
	t.Parallel()

	spec := addSpec()
	spec.Count = 1
	spec.Ephemeral = true
	spec.DisableUpdate = true
	spec.RunnerGroup = ""
	spec.Labels = nil
	spec.RunAsUser = "runner"

	p, err := setup.PlanAdd(spec)
	if err != nil {
		t.Fatalf("err = %v", err)
	}

	lines := p.Units[0].CommandLines()
	cfg := lines[0]
	for _, want := range []string{"--unattended", "--ephemeral", "--disableupdate"} {
		if !strings.Contains(cfg, want) {
			t.Errorf("config.sh に %q が無い: %s", want, cfg)
		}
	}
	if strings.Contains(cfg, "--runnergroup") {
		t.Errorf("runner group 未指定なのに --runnergroup が付いている: %s", cfg)
	}
	if strings.Contains(cfg, "--labels") {
		t.Errorf("ラベル未指定なのに --labels が付いている: %s", cfg)
	}
	if lines[1] != "./svc.sh install runner" {
		t.Errorf("svc.sh install = %q, want 実行ユーザーを渡すこと", lines[1])
	}
}

func TestPlanAddRejectsBadInput(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		mutate func(*setup.AddSpec)
		want   error
	}{
		"台数が 0":                 {func(s *setup.AddSpec) { s.Count = 0 }, valid.ErrBadCount},
		"台数が多すぎる":               {func(s *setup.AddSpec) { s.Count = 999 }, valid.ErrBadCount},
		"GitHub 以外の URL":        {func(s *setup.AddSpec) { s.URL = "https://example.test/foo" }, valid.ErrNotGitHub},
		"http は拒否":              {func(s *setup.AddSpec) { s.URL = "http://github.com/foo" }, valid.ErrNotGitHub},
		"インストール先が相対パス":          {func(s *setup.AddSpec) { s.InstallBase = "runners" }, valid.ErrNotAbs},
		"インストール先に .. を含む":       {func(s *setup.AddSpec) { s.InstallBase = "/opt/../etc" }, valid.ErrHasDotDot},
		"接頭辞が - で始まる":           {func(s *setup.AddSpec) { s.NamePrefix = "-x" }, valid.ErrLeadingDash},
		"接頭辞が空":                 {func(s *setup.AddSpec) { s.NamePrefix = "" }, valid.ErrEmptyName},
		"ラベルが - で始まる":           {func(s *setup.AddSpec) { s.Labels = []string{"-rf"} }, valid.ErrLeadingDash},
		"予約ラベル":                 {func(s *setup.AddSpec) { s.Labels = []string{"self-hosted"} }, valid.ErrReservedLabel},
		"ラベルに使えない文字":            {func(s *setup.AddSpec) { s.Labels = []string{"a b"} }, valid.ErrBadLabelChar},
		"runner group が - で始まる": {func(s *setup.AddSpec) { s.RunnerGroup = "-g" }, valid.ErrLeadingDash},
		"実行ユーザーが - で始まる":        {func(s *setup.AddSpec) { s.RunAsUser = "-u" }, valid.ErrLeadingDash},
		"work dir が - で始まる":     {func(s *setup.AddSpec) { s.WorkDir = "-w" }, valid.ErrLeadingDash},
		"既存の名前と重複": {func(s *setup.AddSpec) {
			s.Existing = []string{"build01-5"}
			s.StartIndex = 5
		}, valid.ErrDuplicateName},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			spec := addSpec()
			tt.mutate(&spec)
			_, err := setup.PlanAdd(spec)
			if !errors.Is(err, tt.want) {
				t.Errorf("err = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestPlanAddWarnsAboutTokenInProcessArgs(t *testing.T) {
	t.Parallel()

	spec := addSpec()
	spec.Busy = []string{"build01-1"}

	p, err := setup.PlanAdd(spec)
	if err != nil {
		t.Fatalf("err = %v", err)
	}

	joined := strings.Join(p.Warnings, "\n")
	if !strings.Contains(joined, "プロセス引数") {
		t.Errorf("トークンがプロセス引数から読める旨の警告が無い:\n%s", joined)
	}
	if !strings.Contains(joined, "build01-1") {
		t.Errorf("ジョブ実行中の runner 名が警告に無い:\n%s", joined)
	}

	quiet, err := setup.PlanAdd(addSpec())
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(quiet.Warnings) != 0 {
		t.Errorf("ジョブ実行中が無いのに警告が出ている: %v", quiet.Warnings)
	}
}

// 1 台ずつのウィザード追加は入力した名前をそのまま使う（FR-12）。
//
// 連番の規則（FR-11）を通すと `gpu-box` が `gpu-box-1` になり、指定した名前で
// 登録されない。
func TestPlanAddUsesExplicitNamesVerbatim(t *testing.T) {
	t.Parallel()

	spec := addSpec()
	spec.Names = []string{"gpu-box"}

	p, err := setup.PlanAdd(spec)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if want := []string{"gpu-box"}; !slices.Equal(p.Names(), want) {
		t.Errorf("名前 = %v, want %v", p.Names(), want)
	}
	if want := []string{"/opt/runners/gpu-box"}; !slices.Equal(p.Dirs(), want) {
		t.Errorf("ディレクトリ = %v, want %v", p.Dirs(), want)
	}
	if !strings.Contains(p.Units[0].CommandLines()[0], "--name gpu-box ") {
		t.Errorf("config.sh に指定した名前が渡っていない: %s", p.Units[0].CommandLines()[0])
	}
}

// 名前を明示した場合も既存との重複と文字種は検証する。
func TestPlanAddValidatesExplicitNames(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		names    []string
		existing []string
		want     error
	}{
		"既存と重複":    {[]string{"build01-1"}, []string{"build01-1"}, valid.ErrDuplicateName},
		"先頭が -":    {[]string{"-rf"}, nil, valid.ErrLeadingDash},
		"使えない文字":   {[]string{"a b"}, nil, valid.ErrBadNameChar},
		"指定どうしで重複": {[]string{"dup", "dup"}, nil, valid.ErrDuplicateName},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			spec := addSpec()
			spec.Names = tt.names
			spec.Existing = tt.existing

			if _, err := setup.PlanAdd(spec); !errors.Is(err, tt.want) {
				t.Errorf("err = %v, want %v", err, tt.want)
			}
		})
	}
}
