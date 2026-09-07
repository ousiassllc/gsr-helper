package buildconfig

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/ousiassllc/gsr-helper/internal/buildconfig/buildconfigtest"
)

// commitSHA はアクションの参照がフルコミット SHA であることの判定に使う。
var commitSHA = regexp.MustCompile(`^[0-9a-f]{40}$`)

// `.github/workflows/ci.yml` の全ジョブに `timeout-minutes` がある。
// ハングしたジョブは GitHub 既定の 6 時間まで実行枠を占有し、その間は同時実行の上限を
// 埋めて後続の run を待たせる。
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

// `.github/workflows/ci.yml` のアクションはフルコミット SHA でピン留めされている。
// 可変タグはタグの移動やアカウント侵害で別のコードに差し替わる。差し替わったコードは
// checkout したソースとビルド成果物、および run に渡る `GITHUB_TOKEN` に触れる。
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

// `.github/workflows/ci.yml` の `actions/checkout` が認証情報を作業ディレクトリへ残さない。
// CI のどの step も認証付きの git 操作を必要としないため、置く理由が無い。既定の true は
// post-job cleanup での削除に依存するが、`cancel-in-progress` によるキャンセルでは
// そこまで完走しない可能性がある。
func TestCICheckoutDoesNotPersistCredentials(t *testing.T) {
	assertWithValue(t, "actions/checkout", "persist-credentials", false)
}

// `.github/workflows/ci.yml` の `actions/setup-go` のキャッシュは無効である。
//
// ホストランナーは run ごとに破棄されるため、self-hosted 時代の「キャッシュはホストに残るので
// 保存と展開が重複でしかない」という理由（docs/environment/setup.md の改訂 1.15）は失効している。
// それでも無効のまま据え置くのは、**有効化が所要時間の計測を伴う別の判断**だからである。
// public リポジトリの標準ランナーは無料なので、短縮は費用に効かない。
func TestCISetupGoDisablesCache(t *testing.T) {
	assertWithValue(t, "actions/setup-go", "cache", false)
}

// `.github/workflows/ci.yml` の `concurrency` が他ワークフローと衝突せず、`main` への push の run を
// キャンセルしない。
//
// group にワークフロー名が入っていないと、別のワークフローの run が同じ group へ入って互いを
// キャンセルし合う。cancel-in-progress を pull_request に限らないと、**main への連続した push で
// 中間コミットの CI 結果が 1 つも残らない**——後から「どのコミットで壊れたか」を run から辿れなく
// なり、二分探索の足場が消える。
//
// 待ち時間を削ろうとして cancel-in-progress を広げる変更は自然に出てくる（self-hosted の
// 稼働台数を空ける動機からこの見直しが起きた経緯は docs/environment/setup.md の改訂 1.6 と
// Issue #16）。**どちらの誤りも CI は緑になる**——run が消えることと run が失敗することは
// 別だからである。
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

// `lefthook.yml` の構文退行を CI でも
// 機械検知する。`make check` の `test` も `TestLefthookConfigIsValid` を通して
// 同じ `go tool lefthook validate` を走らせており、CI の step はそれと重ねて掛ける二重化である。
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
//
// TestCIActionsArePinnedToCommitSHA がアクションをフルコミット SHA に固定しているが、
// **SHA は自分では動かない**。追従する仕組みが無いと、ピン留めの意味は「再現する」から
// 「古いまま凍る」へ静かに変わり、脆弱性修正の入った版が出ても誰も気付かない。
//
// つまりこの 2 つは対で 1 つの取り決めであり、**片方だけが落ちる形になっている**——
// dependabot.yml から github-actions のエントリを外しても、ピン留め側の検査を含め CI は
// すべて緑のままである。ここで落とさないと、気付くのはピン留めした版に問題が見つかった
// ときになる。
func TestDependabotWatchesGitHubActions(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(buildconfigtest.RepoRoot(t), ".github", "dependabot.yml"))
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
