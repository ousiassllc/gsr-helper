package buildconfig

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// commitSHA はアクションの参照がフルコミット SHA であることの判定に使う。
var commitSHA = regexp.MustCompile(`^[0-9a-f]{40}$`)

// ハングしたジョブが runner を GitHub 既定の 6 時間まで占有しないようにする。
// オンラインの runner が 1 台のとき、1 ジョブのハングが CI 全体を止める。
func TestCIJobsHaveTimeout(t *testing.T) {
	for name, job := range loadCIWorkflow(t).Jobs {
		if job.TimeoutMinutes == nil {
			t.Errorf("ジョブ %q に timeout-minutes が無い", name)
			continue
		}
		if *job.TimeoutMinutes <= 0 {
			t.Errorf("ジョブ %q の timeout-minutes が %d", name, *job.TimeoutMinutes)
		}
	}
}

// アクションはフルコミット SHA で固定する。可変タグはタグの移動やアカウント侵害で
// 別のコードに差し替わり、self-hosted runner ではその被害が root 相当まで増幅する。
func TestCIActionsArePinnedToCommitSHA(t *testing.T) {
	pinned := 0
	forEachStep(t, func(job, _ string, step ciStep) {
		if step.Uses == "" {
			return
		}
		_, ref, found := strings.Cut(step.Uses, "@")
		if !found || !commitSHA.MatchString(ref) {
			t.Errorf("ジョブ %q の uses が SHA でピン留めされていない: %s", job, step.Uses)
			return
		}
		pinned++
	})
	if pinned == 0 {
		t.Fatal("uses: を持つ step が 1 つも無い（テストの前提が崩れている）")
	}
}

// checkout が git config に残す認証情報を持ち越さない。self-hosted runner は
// 作業ディレクトリを再利用し、キャンセル時は post-job cleanup が完走しない。
func TestCICheckoutDoesNotPersistCredentials(t *testing.T) {
	assertWithValue(t, "actions/checkout", "persist-credentials", false)
}

// setup-go のキャッシュは無効にする。self-hosted runner ではモジュール・ビルド
// キャッシュがホストに残るため、tar での保存と展開はやり直しの重複でしかない。
func TestCISetupGoDisablesCache(t *testing.T) {
	assertWithValue(t, "actions/setup-go", "cache", false)
}

// concurrency は他ワークフローと衝突せず、main への push をキャンセルしない。
func TestCIConcurrencyIsScopedAndKeepsPushRuns(t *testing.T) {
	got := loadCIWorkflow(t).Concurrency

	if !strings.Contains(got.Group, "github.workflow") {
		t.Errorf("concurrency.group にワークフロー名が含まれていない: %q", got.Group)
	}
	cancel := fmt.Sprint(got.CancelInProgress)
	if !strings.Contains(cancel, "pull_request") {
		t.Errorf("cancel-in-progress が pull_request に限定されていない: %q（main の push で先行 run がキャンセルされる）", cancel)
	}
}

// lefthook.yml の構文退行を CI でも機械検知する。
func TestCILintJobValidatesLefthookConfig(t *testing.T) {
	job, ok := loadCIWorkflow(t).Jobs["lint"]
	if !ok {
		t.Fatal("ci.yml に lint ジョブが無い")
	}
	for _, step := range job.Steps {
		if strings.Contains(step.Run, "lefthook validate") {
			return
		}
	}
	t.Error("lint ジョブに lefthook validate の step が無い")
}

// SHA ピン留めした版へ追従するため、Dependabot の github-actions を有効にする。
func TestDependabotWatchesGitHubActions(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(repoRoot(t), ".github", "dependabot.yml"))
	if err != nil {
		t.Fatalf("dependabot.yml を読めない: %v", err)
	}

	var cfg struct {
		Version int `yaml:"version"`
		Updates []struct {
			PackageEcosystem string `yaml:"package-ecosystem"`
		} `yaml:"updates"`
	}
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		t.Fatalf("dependabot.yml を解析できない: %v", err)
	}

	for _, u := range cfg.Updates {
		if u.PackageEcosystem == "github-actions" {
			return
		}
	}
	t.Error("dependabot.yml に github-actions エコシステムの設定が無い")
}

// forEachStep は ci.yml の全ジョブの全 step を走査する。
func forEachStep(t *testing.T, fn func(jobName, stepName string, step ciStep)) {
	t.Helper()

	for jobName, job := range loadCIWorkflow(t).Jobs {
		for _, step := range job.Steps {
			fn(jobName, step.Name, step)
		}
	}
}

// assertWithValue は指定アクションの with: が期待値になっていることを確かめる。
func assertWithValue(t *testing.T, action, key string, want any) {
	t.Helper()

	found := 0
	forEachStep(t, func(job, _ string, step ciStep) {
		if !strings.HasPrefix(step.Uses, action+"@") {
			return
		}
		found++
		got, ok := step.With[key]
		if !ok {
			t.Errorf("ジョブ %q の %s に with.%s が無い（期待: %v）", job, action, key, want)
			return
		}
		if got != want {
			t.Errorf("ジョブ %q の %s の with.%s が %v（期待: %v）", job, action, key, got, want)
		}
	})
	if found == 0 {
		t.Fatalf("%s を使う step が無い（テストの前提が崩れている）", action)
	}
}
