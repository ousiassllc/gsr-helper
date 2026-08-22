package disk

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// newTargetTree は削除計画の検証に使う runner ディレクトリを作る。
func newTargetTree(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	for _, sub := range []string{
		filepath.Join("_work", "_temp"),
		"_diag",
		"bin",
	} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0o755); err != nil {
			t.Fatalf("%s の作成に失敗した: %v", sub, err)
		}
	}
	return dir
}

// pathTarget はファイル削除の対象を組み立てる。
func pathTarget(base, rel string, bytes int64) Target {
	return Target{
		Label:  "build01-1 / " + rel,
		Base:   base,
		Path:   filepath.Join(base, rel),
		Bytes:  bytes,
		Files:  1,
		Docker: false,
	}
}

// dockerTarget は docker の未使用リソースの対象を組み立てる。
func dockerTarget(bytes int64) Target {
	return Target{Label: dockerCleanLabel, Base: "", Path: "", Bytes: bytes, Files: -1, Docker: true}
}

func TestPlanClean(t *testing.T) {
	dir := newTargetTree(t)
	temp := pathTarget(dir, filepath.Join("_work", "_temp"), 1200)
	diag := pathTarget(dir, "_diag", 100)

	plan, err := PlanClean([]Target{temp, diag, dockerTarget(12400), dockerTarget(3100)})
	if err != nil {
		t.Fatalf("PlanClean がエラーを返した: %v", err)
	}

	if !slices.Equal(plan.Paths, []Target{temp, diag}) {
		t.Errorf("Paths = %+v, want %+v", plan.Paths, []Target{temp, diag})
	}
	if !plan.Docker {
		t.Error("Docker = false, want true")
	}
	if want := int64(1200 + 100 + 12400 + 3100); plan.Bytes != want {
		t.Errorf("Bytes = %d, want %d", plan.Bytes, want)
	}
	// docker の対象が 2 件あっても発行するコマンドは 1 本だけ。
	// ファイル削除は外部コマンドを使わないので Commands には現れない。
	want := [][]string{{"docker", "system", "prune", "-f"}}
	if !slices.EqualFunc(plan.Commands, want, slices.Equal) {
		t.Errorf("Commands = %v, want %v", plan.Commands, want)
	}
}

// TestPlanCleanWithoutDocker は docker を選ばなければコマンドを出さないことを確かめる。
func TestPlanCleanWithoutDocker(t *testing.T) {
	dir := newTargetTree(t)

	plan, err := PlanClean([]Target{pathTarget(dir, "_diag", 100)})
	if err != nil {
		t.Fatalf("PlanClean がエラーを返した: %v", err)
	}
	if plan.Docker || len(plan.Commands) != 0 {
		t.Errorf("Docker = %v, Commands = %v, want false と 0 件", plan.Docker, plan.Commands)
	}
}

func TestPlanCleanErrors(t *testing.T) {
	dir := newTargetTree(t)

	tests := []struct {
		name    string
		targets []Target
		wantMsg string
	}{
		{name: "対象なし", targets: nil, wantMsg: "対象が選択されていません"},
		{
			name:    "許可サブツリーの外",
			targets: []Target{pathTarget(dir, "bin", 10)},
			wantMsg: "削除が許可されたサブツリー",
		},
		{
			// 1 件でも検証を通らなければ計画そのものを作らない。通った分だけ返すと、
			// 確認画面に出ていない対象が残ったことに利用者が気付けない。
			name: "検証を通る対象と混在",
			targets: []Target{
				pathTarget(dir, "_diag", 10),
				// filepath.Join に畳ませず ".." を残した生のパス。
				{Label: "build01-1 / 相対参照", Base: dir, Path: dir + "/_work/../bin", Bytes: 10, Files: 1, Docker: false},
			},
			wantMsg: `".." が含まれています`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan, err := PlanClean(tt.targets)
			if err == nil {
				t.Fatalf("PlanClean が計画を作ってしまった: %+v", plan)
			}
			if !strings.Contains(err.Error(), tt.wantMsg) {
				t.Errorf("エラー文言 = %q, want %q を含む", err.Error(), tt.wantMsg)
			}
			if len(plan.Paths) != 0 || plan.Docker || plan.Bytes != 0 || len(plan.Commands) != 0 {
				t.Errorf("失敗時の CleanPlan = %+v, want ゼロ値", plan)
			}
		})
	}
}
