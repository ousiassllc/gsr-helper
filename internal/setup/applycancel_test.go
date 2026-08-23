package setup_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/exec"
	"github.com/ousiassllc/gsr-helper/internal/setup"
)

// 台と台の境目で終了要求を受けた場合、着手済みの台は成功のまま残し、次の台は
// 「失敗」ではなく未着手として Remaining に載せる（FR-15）。
//
// 台の途中でキャンセルする既存のテストでは applyUnit 側の判定が先に働くため、
// Apply のループ先頭にある判定は一度も効かない。ここでは 1 台目の完了通知を
// 受けた時点でキャンセルし、その判定だけが働く状況を作る。
func TestApplyStopsBetweenUnitsAndReportsNextAsRemaining(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	ctx, cancel := context.WithCancel(t.Context())

	f := exec.NewFake()
	res, err := setup.Apply(ctx, setup.ApplyInput{
		Exec:     f,
		Plan:     addPlanIn(t, base, 2),
		Token:    "TOKENTOKENTOKEN",
		TokenFor: nil,
		Tarball:  makeTarball(t),
		Drain:    nil,
		// 1 台目が最後まで終わった直後（Done かつ失敗なし）に終了要求が来た状況。
		Progress: func(p setup.Progress) {
			if p.Done && p.Err == nil {
				cancel()
			}
		},
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}

	if want := []string{"build01-5"}; !slices.Equal(res.Succeeded, want) {
		t.Errorf("成功 = %v, want %v", res.Succeeded, want)
	}
	if want := []string{"build01-6"}; !slices.Equal(res.Remaining, want) {
		t.Errorf("未着手 = %v, want %v", res.Remaining, want)
	}
	// 手を付けていない台を、壊したように見せてはいけない。
	if res.Failed != "" {
		t.Errorf("失敗した台 = %q, want 空（2 台目は着手していない）", res.Failed)
	}
	if res.Phase != "" {
		t.Errorf("失敗フェーズ = %q, want 空（2 台目は着手していない）", res.Phase)
	}

	for _, c := range f.Calls() {
		if c.Options.Runner != "build01-5" {
			t.Errorf("未着手の台にコマンドを発行している: %v", c)
		}
	}
	// 最初の手順（ディレクトリ作成）にすら入っていないこと。
	if _, serr := os.Stat(filepath.Join(base, "build01-6")); !errors.Is(serr, os.ErrNotExist) {
		t.Errorf("未着手の台のディレクトリを作っている: %v", serr)
	}
}
