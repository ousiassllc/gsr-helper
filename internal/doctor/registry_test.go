package doctor_test

import (
	"slices"
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/doctor"
	"github.com/ousiassllc/gsr-helper/internal/doctor/check"
)

// 識別子は一意でなければならない。
//
// 個別の再実行（FR-34）は識別子で項目を引く（doctor.ByID）ので、重複すると
// 1 つ再実行したつもりで別の項目まで走る。
func TestDefaultIDsAreUnique(t *testing.T) {
	t.Parallel()

	seen := make(map[string]bool)
	for _, c := range doctor.Default() {
		if seen[c.ID()] {
			t.Errorf("識別子 %q が重複している", c.ID())
		}
		seen[c.ID()] = true
	}
	if len(seen) == 0 {
		t.Fatal("項目が 1 つも登録されていない")
	}
}

// functional.md のチェック項目一覧表の分類をすべて実装している。
//
// 分類を 1 つでも取りこぼすと、その分類の不備が診断に出ないまま「OK 20 件」と
// 表示される。件数だけを見た運用者は確認済みと受け取るので、抜けは緑より悪い。
func TestDefaultCoversEveryCategory(t *testing.T) {
	t.Parallel()

	implemented := make(map[string]bool)
	for _, c := range doctor.Default() {
		implemented[c.Category()] = true
	}

	for _, cat := range doctor.Categories() {
		if !implemented[cat] {
			t.Errorf("分類 %q の項目が 1 つも無い", cat)
		}
	}
}

// 分類は check が定めたもののいずれかでなければならない。
// 表記のゆれた分類を作ると、並び順（CategoryRank）で末尾へ落ちる。
func TestDefaultUsesKnownCategories(t *testing.T) {
	t.Parallel()

	known := doctor.Categories()
	for _, c := range doctor.Default() {
		if !slices.Contains(known, c.Category()) {
			t.Errorf("%s の分類 %q は check が定めた分類にない", c.ID(), c.Category())
		}
	}
}

// 起動時の自動判定（FR-44）の対象は「ジョブ実行の前提」の 4 点だけである。
//
// ネットワーク到達性やディスク集計を含めると、起動が回線とディスクの状態に
// 引きずられる（data-model.md の CheckResult.Startup）。
func TestStartupSetIsLimitedToJobRequirements(t *testing.T) {
	t.Parallel()

	var ids []string
	for _, c := range doctor.Startup(doctor.Default()) {
		ids = append(ids, c.ID())
		if got := c.Category(); got != check.CatJobReq {
			t.Errorf("%s の分類は %q で、ジョブ実行の前提ではない", c.ID(), got)
		}
	}
	slices.Sort(ids)

	want := []string{"job.buildx", "job.docker", "job.dockergroup", "job.sudo"}
	if !slices.Equal(ids, want) {
		t.Errorf("起動時の対象 = %q, want %q", ids, want)
	}
}

// 個別の再実行はレジストリから 1 項目だけを引ける。
func TestByIDOnDefault(t *testing.T) {
	t.Parallel()

	got := doctor.ByID(doctor.Default(), "job.dockergroup")
	if len(got) != 1 {
		t.Fatalf("件数 = %d, want 1", len(got))
	}
	if got[0].ID() != "job.dockergroup" {
		t.Errorf("ID = %q, want %q", got[0].ID(), "job.dockergroup")
	}
}
