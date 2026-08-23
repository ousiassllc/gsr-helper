package disk

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/exec"
)

// wantPruneArgs は docker の未使用リソース削除で発行するコマンド列。
// 破壊的な docker コマンドはこれ 1 本だけに固定する。
var wantPruneArgs = []string{"system", "prune", "-f"}

// recorder は progress の通知を記録する。
type recorder struct {
	got []Progress
}

func (r *recorder) record(p Progress) { r.got = append(r.got, p) }

// exists はパスが存在するかを返す（リンクは辿らない）。
func exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

func TestApplyRemovesTree(t *testing.T) {
	dir := newScanTree(t)
	temp := filepath.Join(dir, "_work", "_temp")
	mkFile(t, filepath.Join(temp, "deep", "c.txt"), 5)

	plan, err := PlanClean([]Target{
		pathTarget(dir, filepath.Join("_work", "repo"), 150),
		pathTarget(dir, filepath.Join("_work", "_temp"), 5),
	})
	if err != nil {
		t.Fatalf("PlanClean がエラーを返した: %v", err)
	}

	var rec recorder
	if aerr := Apply(context.Background(), exec.NewFake(), nil, plan, rec.record); aerr != nil {
		t.Fatalf("Apply がエラーを返した: %v", aerr)
	}

	for _, path := range []string{filepath.Join(dir, "_work", "repo"), temp} {
		if exists(path) {
			t.Errorf("%s が残っている", path)
		}
	}
	// 対象ごとに 1 回、Done が 1 から Total まで進む。
	want := []Progress{
		{Label: "build01-1 / _work/repo", Done: 1, Total: 2, Err: nil},
		{Label: "build01-1 / _work/_temp", Done: 2, Total: 2, Err: nil},
	}
	if !slices.Equal(rec.got, want) {
		t.Errorf("Progress\n got: %+v\nwant: %+v", rec.got, want)
	}
}

// TestApplyDoesNotFollowSymlink はリンクを辿らずリンク自体だけを消すことを確かめる
// （docs/architecture/security.md「シンボリックリンクの扱い」）。取り違えると
// _work 内のリンクが指す外部のファイルまで消える。
func TestApplyDoesNotFollowSymlink(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "runner")
	outside := filepath.Join(root, "outside")
	mkFile(t, filepath.Join(outside, "keep.txt"), 4)

	repo := filepath.Join(dir, "_work", "repo")
	mkFile(t, filepath.Join(repo, "a.txt"), 1)
	if err := os.Symlink(outside, filepath.Join(repo, "link")); err != nil {
		t.Fatalf("シンボリックリンクの作成に失敗した: %v", err)
	}

	plan, err := PlanClean([]Target{pathTarget(dir, filepath.Join("_work", "repo"), 1)})
	if err != nil {
		t.Fatalf("PlanClean がエラーを返した: %v", err)
	}
	if aerr := Apply(context.Background(), exec.NewFake(), nil, plan, nil); aerr != nil {
		t.Fatalf("Apply がエラーを返した: %v", aerr)
	}

	if exists(repo) {
		t.Errorf("%s が残っている", repo)
	}
	if !exists(filepath.Join(outside, "keep.txt")) {
		t.Error("リンク先の外部ファイルを消してしまった")
	}
}

// TestApplyValidatesBeforeRemoving は計画を経ずに組み立てた CleanPlan でも
// 削除直前の検証が働くことを確かめる（受け入れ条件）。
func TestApplyValidatesBeforeRemoving(t *testing.T) {
	dir := newTargetTree(t)
	mkFile(t, filepath.Join(dir, "bin", "runsvc.sh"), 3)
	bad := Target{
		Label: "build01-1 / bin", Base: dir, Path: filepath.Join(dir, "bin"),
		Bytes: 3, Files: 1, Docker: false, Protected: "",
	}
	good := pathTarget(dir, "_diag", 0)

	// PlanClean を通さずに直接組み立てた計画。検証を通っていないパスが載っている。
	plan := CleanPlan{Paths: []Target{bad, good}, Docker: false, Bytes: 3, Commands: nil}

	var rec recorder
	err := Apply(context.Background(), exec.NewFake(), nil, plan, rec.record)
	if err == nil {
		t.Fatal("Apply がエラーを返さなかった")
	}
	if !strings.Contains(err.Error(), "削除が許可されたサブツリー") {
		t.Errorf("エラー文言 = %q, want 検証で落ちたことが分かる文言", err.Error())
	}
	if !exists(filepath.Join(dir, "bin", "runsvc.sh")) {
		t.Error("検証を通らないパスを削除してしまった")
	}
	// 1 件の失敗で残りを止めない。
	if exists(filepath.Join(dir, "_diag")) {
		t.Error("後続の対象が削除されていない")
	}
	if len(rec.got) != 2 || rec.got[0].Err == nil || rec.got[1].Err != nil {
		t.Errorf("Progress = %+v, want 2 件で 1 件目だけ Err", rec.got)
	}
}

// TestApplyRefusesProtectedTarget は保護された対象を Apply が削除しないことを
// 確かめる（FR-31）。PlanClean を迂回して手組みした CleanPlan を渡すのは、保護が
// 計画の段階だけの約束になっていないか（disk.Target.Protected の doc）を見るためで
// ある。TestApplyValidatesBeforeRemoving と同じ形。
func TestApplyRefusesProtectedTarget(t *testing.T) {
	dir := newTargetTree(t)
	temp := filepath.Join(dir, "_work", "_temp")
	mkFile(t, filepath.Join(temp, "a.txt"), 3)
	busy := protectedTarget(dir, filepath.Join("_work", "_temp"))
	good := pathTarget(dir, "_diag", 0)

	plan := CleanPlan{Paths: []Target{busy, good}, Docker: false, Bytes: 100, Commands: nil}

	var rec recorder
	err := Apply(context.Background(), exec.NewFake(), nil, plan, rec.record)
	if err == nil {
		t.Fatal("Apply がエラーを返さなかった")
	}
	if !strings.Contains(err.Error(), busyReason) {
		t.Errorf("エラー文言 = %q, want %q を含む", err.Error(), busyReason)
	}
	if !exists(filepath.Join(temp, "a.txt")) {
		t.Error("保護された対象を削除してしまった")
	}
	// 1 件の失敗で残りを止めない。
	if exists(filepath.Join(dir, "_diag")) {
		t.Error("後続の対象が削除されていない")
	}
	if len(rec.got) != 2 || rec.got[0].Err == nil || rec.got[1].Err != nil {
		t.Errorf("Progress = %+v, want 2 件で 1 件目だけ Err", rec.got)
	}
}

func TestApplyPrunesDocker(t *testing.T) {
	f := exec.NewFake()
	f.Push(exec.Result{Stdout: []byte("Total reclaimed space: 12.4GB\n"), Stderr: nil, ExitCode: 0}, nil)

	plan, err := PlanClean([]Target{dockerTarget(12400)})
	if err != nil {
		t.Fatalf("PlanClean がエラーを返した: %v", err)
	}

	var rec recorder
	if aerr := Apply(context.Background(), f, nil, plan, rec.record); aerr != nil {
		t.Fatalf("Apply がエラーを返した: %v", aerr)
	}

	calls := f.Calls()
	if len(calls) != 1 {
		t.Fatalf("発行コマンド数 = %d, want 1（%v）", len(calls), calls)
	}
	c := calls[0]
	if c.Name != "docker" || !slices.Equal(c.Args, wantPruneArgs) {
		t.Errorf("発行コマンド = %q, want docker %v", c.String(), wantPruneArgs)
	}
	// 破壊的操作は監査ログに全件残す。action は disk.clean で固定。
	if c.Options.Action != "disk.clean" || c.Options.SkipAudit {
		t.Errorf("Options = %+v, want Action=disk.clean SkipAudit=false", c.Options)
	}
	want := []Progress{{Label: dockerCleanLabel, Done: 1, Total: 1, Err: nil}}
	if !slices.Equal(rec.got, want) {
		t.Errorf("Progress\n got: %+v\nwant: %+v", rec.got, want)
	}
}

// TestApplyDockerFailure は prune の失敗をエラーとして返すことを確かめる。
func TestApplyDockerFailure(t *testing.T) {
	f := exec.NewFake()
	f.Push(exec.Result{Stdout: nil, Stderr: []byte("daemon 不応答"), ExitCode: 1}, nil)

	plan := CleanPlan{Paths: nil, Docker: true, Bytes: 0, Commands: [][]string{pruneCommand}}
	err := Apply(context.Background(), f, nil, plan, nil)
	if err == nil {
		t.Fatal("Apply がエラーを返さなかった")
	}
	if !strings.Contains(err.Error(), "終了コード 1") {
		t.Errorf("エラー文言 = %q, want 終了コードが分かる文言", err.Error())
	}
}

// TestApplyCanceled はキャンセル後に残りの対象へ手を付けないことを確かめる。
func TestApplyCanceled(t *testing.T) {
	dir := newTargetTree(t)
	mkFile(t, filepath.Join(dir, "_diag", "d.log"), 2)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	plan := CleanPlan{
		Paths:    []Target{pathTarget(dir, "_diag", 2)},
		Docker:   false,
		Bytes:    2,
		Commands: nil,
	}
	if err := Apply(ctx, exec.NewFake(), nil, plan, nil); err == nil {
		t.Fatal("Apply がエラーを返さなかった")
	}
	if !exists(filepath.Join(dir, "_diag", "d.log")) {
		t.Error("キャンセル後に削除してしまった")
	}
}
