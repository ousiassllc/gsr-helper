package buildconfig

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/ousiassllc/gsr-helper/internal/buildconfig/buildconfigtest"
)

// guardJobName は self-hosted runner を使うジョブの前段に置くゲートジョブ名。
const guardJobName = "guard"

// yamlStrings は string / []string のどちらでも書ける YAML フィールドを受ける。
type yamlStrings []string

func (s *yamlStrings) UnmarshalYAML(node *yaml.Node) error {
	var one string
	if err := node.Decode(&one); err == nil {
		*s = yamlStrings{one}
		return nil
	}
	var many []string
	if err := node.Decode(&many); err != nil {
		return err
	}
	*s = many
	return nil
}

type ciStep struct {
	Name string            `yaml:"name"`
	Env  map[string]string `yaml:"env"`
	Run  string            `yaml:"run"`
	Uses string            `yaml:"uses"`
	With map[string]any    `yaml:"with"`
}

type ciJob struct {
	Needs          yamlStrings `yaml:"needs"`
	RunsOn         yamlStrings `yaml:"runs-on"`
	TimeoutMinutes *int        `yaml:"timeout-minutes"`
	Steps          []ciStep    `yaml:"steps"`
}

type ciConcurrency struct {
	Group            string `yaml:"group"`
	CancelInProgress any    `yaml:"cancel-in-progress"`
}

// ciWorkflow は検証に使うフィールドだけを読む。`on:` は YAML の真偽値と衝突する
// ため構造体では扱わず、トリガーの検証は生のテキストに対して行う。
type ciWorkflow struct {
	Concurrency ciConcurrency    `yaml:"concurrency"`
	Jobs        map[string]ciJob `yaml:"jobs"`
}

func workflowsDir(t *testing.T) string {
	t.Helper()
	return filepath.Join(buildconfigtest.RepoRoot(t), ".github", "workflows")
}

func loadCIWorkflow(t *testing.T) ciWorkflow {
	t.Helper()

	raw, err := os.ReadFile(filepath.Join(workflowsDir(t), "ci.yml"))
	if err != nil {
		t.Fatalf("ci.yml を読めない: %v", err)
	}
	var wf ciWorkflow
	if err := yaml.Unmarshal(raw, &wf); err != nil {
		t.Fatalf("ci.yml を解析できない: %v", err)
	}
	if len(wf.Jobs) == 0 {
		t.Fatal("ci.yml に jobs が無い")
	}
	return wf
}

func isSelfHosted(runsOn yamlStrings) bool {
	return slices.Contains(runsOn, "self-hosted")
}

// `guard` ジョブ自身は GitHub ホストランナーで動く。ゲートを self-hosted 上で
// 走らせると、fork の PR がゲート自身をホスト上で実行できてしまい、ゲートを置く意味が無い。
func TestCIGuardJobDoesNotUseSelfHostedRunner(t *testing.T) {
	wf := loadCIWorkflow(t)

	guard, ok := wf.Jobs[guardJobName]
	if !ok {
		t.Fatalf("ci.yml に %q ジョブが無い", guardJobName)
	}
	if isSelfHosted(guard.RunsOn) {
		t.Errorf("%q ジョブが self-hosted runner を使っている: %v", guardJobName, guard.RunsOn)
	}
	if len(guard.RunsOn) == 0 {
		t.Errorf("%q ジョブに runs-on が無い", guardJobName)
	}
}

// `.github/workflows/ci.yml` の self-hosted runner を使うジョブは、必ず `needs: guard` で
// ゲートジョブに依存する。`if:` による skip では required status check に対して success 扱いに
// なり、マージを機械的に止められない。
func TestCISelfHostedJobsDependOnGuard(t *testing.T) {
	wf := loadCIWorkflow(t)

	selfHosted := 0
	for name, job := range wf.Jobs {
		if !isSelfHosted(job.RunsOn) {
			continue
		}
		selfHosted++
		if !slices.Contains(job.Needs, guardJobName) {
			t.Errorf("self-hosted ジョブ %q が needs: %s を持たない（needs=%v）", name, guardJobName, job.Needs)
		}
	}
	if selfHosted == 0 {
		t.Fatal("self-hosted runner を使うジョブが 1 つも無い（テストの前提が崩れている）")
	}
}

// `.github/workflows/ci.yml` の `guard` は許可リスト形である（`push` と
// 同一リポジトリの `pull_request` 以外は失敗する）。
// 判定値は `EVENT_NAME` / `HEAD_REPO` / `BASE_REPO` の env 経由で渡り、ゲートの `run:` に `${{` を
// 直書きしない（head リポジトリ名を通した式インジェクションの余地を残さないため）。
// 許可していないトリガーを `on:` に足したときに既定が「実行しない」側へ倒れる必要がある。
func TestCIGuardScriptAllowsOnlySameRepositoryEvents(t *testing.T) {
	script, env := guardScript(t)

	const baseRepo = "ousiassllc/gsr-helper"
	tests := []struct {
		name      string
		eventName string
		headRepo  string
		wantAllow bool
	}{
		{"main への push は実行する", "push", "", true},
		{"同一リポジトリのブランチからの PR は実行する", "pull_request", baseRepo, true},
		{"fork からの PR は実行しない", "pull_request", "attacker/gsr-helper", false},
		{"許可していない workflow_dispatch は実行しない", "workflow_dispatch", "", false},
		{"許可していない merge_group は実行しない", "merge_group", "", false},
		{"head リポジトリが空の PR は実行しない", "pull_request", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := exec.Command("bash", "-e", "-c", script)
			cmd.Env = append(os.Environ(),
				"EVENT_NAME="+tt.eventName,
				"HEAD_REPO="+tt.headRepo,
				"BASE_REPO="+baseRepo,
			)
			out, err := cmd.CombinedOutput()

			if gotAllow := err == nil; gotAllow != tt.wantAllow {
				t.Fatalf("ゲートの判定が期待と違う: allow=%v want=%v\n出力:\n%s", gotAllow, tt.wantAllow, out)
			}
		})
	}

	// 判定に使う値はすべて env 経由で渡す（run: 内へ式を直接埋め込むと
	// head リポジトリ名を通したスクリプトインジェクションの余地が残る）。
	for _, key := range []string{"EVENT_NAME", "HEAD_REPO", "BASE_REPO"} {
		if _, ok := env[key]; !ok {
			t.Errorf("ゲートの step に env %s が無い", key)
		}
	}
	if strings.Contains(script, "${{") {
		t.Errorf("ゲートの run: に GitHub Actions の式が直接埋め込まれている:\n%s", script)
	}
}

// guardScript はゲートジョブの判定スクリプトと、その step の env を返す。
func guardScript(t *testing.T) (string, map[string]string) {
	t.Helper()

	guard, ok := loadCIWorkflow(t).Jobs[guardJobName]
	if !ok {
		t.Fatalf("ci.yml に %q ジョブが無い", guardJobName)
	}
	for _, step := range guard.Steps {
		if step.Run != "" {
			return step.Run, step.Env
		}
	}
	t.Fatalf("%q ジョブに run: を持つ step が無い", guardJobName)
	return "", nil
}

// どのワークフローも `pull_request_target` を使わない。fork の PR に対してベースリポジトリ側の
// 権限でワークフローが動き、fork ガードの意味が失われる。
func TestWorkflowsDoNotUsePullRequestTarget(t *testing.T) {
	entries, err := os.ReadDir(workflowsDir(t))
	if err != nil {
		t.Fatalf("ワークフローディレクトリを読めない: %v", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(workflowsDir(t), entry.Name()))
		if err != nil {
			t.Fatalf("%s を読めない: %v", entry.Name(), err)
		}
		if strings.Contains(string(raw), "pull_request_target") {
			t.Errorf("%s が pull_request_target を使っている", entry.Name())
		}
	}
}
