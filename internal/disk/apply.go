package disk

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/ousiassllc/gsr-helper/internal/exec"
)

// actionClean は破壊的なクリーンアップを表す監査ログの action。
// 破壊的操作は全件記録するため SkipAudit は付けない
// （docs/architecture/security.md「監査ログ」）。
const actionClean = "disk.clean"

// dockerCleanLabel は docker の削除の進捗に出す表示名。
const dockerCleanLabel = "docker / 未使用リソース"

// Apply は削除計画を実行する（FR-30）。
//
// 削除の直前に ValidatePath をもう一度呼ぶ。計画を作ってから実行するまでの間に
// パスの実体が差し替えられる余地を残さないためであり、計画を組み立てずに Apply を
// 呼ぶ経路が将来できても検証を迂回できないようにするためでもある。
//
// 1 件の失敗で残りを止めない。ディスクを空けるために選んだ対象のうち 1 つが
// 権限不足で消せなくても、残りは消せたほうが目的を達する。失敗は errors.Join で
// 束ねて返し、対象ごとの内訳は progress で通知する。progress は nil でもよい。
func Apply(ctx context.Context, ex exec.Executor, plan CleanPlan, progress func(Progress)) error {
	total := len(plan.Paths)
	if plan.Docker {
		total++
	}

	var errs []error
	done := 0
	for _, t := range plan.Paths {
		if err := ctx.Err(); err != nil {
			errs = append(errs, fmt.Errorf("クリーンアップを中断しました: %w", err))
			return errors.Join(errs...)
		}

		err := removeTarget(ctx, t)
		done++
		report(progress, Progress{Label: t.Label, Done: done, Total: total, Err: err})
		if err != nil {
			errs = append(errs, err)
		}
	}

	if plan.Docker {
		err := pruneDocker(ctx, ex)
		done++
		report(progress, Progress{Label: dockerCleanLabel, Done: done, Total: total, Err: err})
		if err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// report は progress が設定されていれば通知する。
func report(progress func(Progress), p Progress) {
	if progress != nil {
		progress(p)
	}
}

// removeTarget は 1 対象を検証してから削除する。
func removeTarget(ctx context.Context, t Target) error {
	if err := ValidatePath(t.Base, t.Path); err != nil {
		return fmt.Errorf("%s の削除を中止しました: %w", t.Label, err)
	}
	if err := removeTree(ctx, t.Path); err != nil {
		return fmt.Errorf("%s の削除に失敗しました: %w", t.Label, err)
	}
	return nil
}

// pruneDocker は docker の未使用リソースを削除する。
func pruneDocker(ctx context.Context, ex exec.Executor) error {
	ctx = exec.WithOptions(ctx, exec.Options{
		Action: actionClean, Runner: "", Dir: "", Env: nil, SkipAudit: false,
	})

	res, err := ex.Run(ctx, pruneCommand[0], pruneCommand[1:]...)
	if err != nil {
		return fmt.Errorf("docker system prune の実行に失敗しました: %w", err)
	}
	if res.ExitCode != 0 {
		return fmt.Errorf("docker system prune が終了コード %d で終了しました", res.ExitCode)
	}
	return nil
}

// removeTree は path を再帰的に削除する。
//
// os.RemoveAll を使わないのは、キャンセルを途中で受け取れるようにするためと、
// エントリごとの判定を自分で持つためである。判定は os.Lstat で行い、シンボリック
// リンクはリンク自体だけを消す。辿ってしまうと _work 内のリンクが指す外部の
// ファイルまで消えかねない（docs/architecture/security.md「シンボリックリンクの扱い」）。
func removeTree(ctx context.Context, path string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("削除を中断しました: %w", err)
	}

	fi, err := os.Lstat(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil // 既に無いものは成功として扱う
		}
		return fmt.Errorf("%s の情報取得に失敗しました: %w", path, err)
	}

	// リンクは IsDir が偽になるが、判定順に依存しないよう先に分岐しておく。
	if fi.Mode()&fs.ModeSymlink != 0 || !fi.IsDir() {
		return removeOne(path)
	}

	entries, err := os.ReadDir(path)
	if err != nil {
		return fmt.Errorf("%s の読み込みに失敗しました: %w", path, err)
	}
	for _, e := range entries {
		if err := removeTree(ctx, filepath.Join(path, e.Name())); err != nil {
			return err
		}
	}
	return removeOne(path)
}

// removeOne は 1 エントリを削除する。リンクは辿らない（os.Remove の仕様）。
func removeOne(path string) error {
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("%s を削除できませんでした: %w", path, err)
	}
	return nil
}
