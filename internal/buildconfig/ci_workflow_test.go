package buildconfig

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/ousiassllc/gsr-helper/internal/buildconfig/buildconfigtest"
)

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
	Name string         `yaml:"name"`
	Run  string         `yaml:"run"`
	Uses string         `yaml:"uses"`
	With map[string]any `yaml:"with"`
}

type ciJob struct {
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

// `.github/workflows/ci.yml` のジョブはすべて GitHub ホストランナーで動く。
//
// **self-hosted runner を使う形へ戻さない。** このリポジトリは public であり、
// `pull_request` はワークフロー定義をマージコミット側（fork の変更を含む側）から取るため、
// fork の PR がホスト上で任意のコードを実行できる形になる。runner 実行ユーザーは
// パスワード不要 sudo を持つ運用が前提（internal/doctor/jobreq がそれを検出する）なので、
// 到達されれば実質 root であり、作業ディレクトリもビルドキャッシュも次のジョブへ残る。
//
// public リポジトリの標準ランナーは無料なので、self-hosted へ戻す費用面の動機も無い
// （docs/environment/setup.md の「CI/CD」）。
func TestCIJobsUseGitHubHostedRunners(t *testing.T) {
	jobs := loadCIWorkflow(t).Jobs

	for name, job := range jobs {
		if len(job.RunsOn) == 0 {
			t.Errorf("ジョブ %q に runs-on が無い", name)
			continue
		}
		if isSelfHosted(job.RunsOn) {
			t.Errorf("ジョブ %q が self-hosted runner を使っている: %v", name, job.RunsOn)
		}
	}
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
