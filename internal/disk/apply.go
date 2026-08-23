package disk

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/ousiassllc/gsr-helper/internal/audit"
	"github.com/ousiassllc/gsr-helper/internal/exec"
)

// actionClean は破壊的なクリーンアップを表す監査ログの action。
// 破壊的操作は全件記録するため SkipAudit は付けない
// （docs/architecture/security.md「監査ログ」）。
//
// ファイル削除（removeTarget）と docker prune（pruneDocker）の両方でこの値を使う。
// 同じ「ディスクのクリーンアップ」という操作を、実行手段（自前の削除か外部
// コマンドか）で action を分けると、監査ログを読む側が同じ操作を 2 つの action で
// 追う羽目になる。
const actionClean = "disk.clean"

// DockerLabel は docker の削除の進捗（Progress.Label）に出る表示名。
//
// **公開しているのは、表示側が名前で突き合わせるためである。** Disk タブは進捗の
// Label で行を引き当てるので（page/disk/cleanview.Mark）、同じ文字列を写しで持つと
// こちらを変えた瞬間に docker の行だけ永久に未着手で残り、報告の件数もずれる。
// コンパイルもテストも通ってしまうため、名前の一致は型で保証する。
const DockerLabel = "docker / 未使用リソース"

// auditRemoveLabel は監査レコードの command[0] に載せる値。
//
// "rm" や "rm -rf" のような実在するコマンド名にしない。ファイルの再帰削除は
// internal/disk が os.Remove を直接呼んで行い、外部コマンドを一切起動しない
// （removeTree の doc）。実在するコマンド名を書くと、監査ログを読む側が
// 「rm を起動した」と誤読し、実行していないコマンドが実行されたことになる。
// 括弧書きにしているのは、コマンド名の語彙（英数字とハイフン）に現れない見た目に
// することで、一覧を眺めただけで「これはコマンドではない」と分かるようにするため。
const auditRemoveLabel = "(削除)"

// Apply は削除計画を実行する（FR-30）。
//
// 削除の直前に ValidatePath をもう一度呼ぶ。計画を作ってから実行するまでの間に
// パスの実体が差し替えられる余地を残さないためであり、計画を組み立てずに Apply を
// 呼ぶ経路が将来できても検証を迂回できないようにするためでもある。
//
// 1 件の失敗で残りを止めない。ディスクを空けるために選んだ対象のうち 1 つが
// 権限不足で消せなくても、残りは消せたほうが目的を達する。失敗は errors.Join で
// 束ねて返し、対象ごとの内訳は progress で通知する。progress は nil でもよい。
//
// lg はファイル削除を監査ログへ記録する Logger。nil / audit.Discard() でも動作は
// 変わらない（従来どおり削除が続く）。監査ログを開けない場合の縮退がここでも
// そのまま働く。
func Apply(ctx context.Context, ex exec.Executor, lg *audit.Logger, plan CleanPlan, progress func(Progress)) error {
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

		err := removeTarget(ctx, lg, t)
		done++
		report(progress, Progress{Label: t.Label, Done: done, Total: total, Err: err})
		if err != nil {
			errs = append(errs, err)
		}
	}

	if plan.Docker {
		err := pruneDocker(ctx, ex)
		done++
		report(progress, Progress{Label: DockerLabel, Done: done, Total: total, Err: err})
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
//
// 保護の確認を ValidatePath と同じ位置で行うのは同じ理由による。保護された対象は
// PlanClean が計画に載せないが、計画を組み立てずに Apply を呼ぶ経路が将来できても
// ジョブ実行中の _work（FR-31）を迂回で消せないようにしておく。
//
// **監査ログの記録の起点はここ 1 か所に固定する。** ファイル削除は internal/exec を
// 通らないため（removeTree は外部コマンドを使わない）、docs/architecture/security.md
// が定める「監査ログ」を満たすには internal/disk 側にも記録の起点を持つ必要がある。
// Apply からの経路をすべて removeTarget に集約している（保護・検証で中止した場合も
// 含めて必ずここを通る）ことで、削除の実装を増やしても記録漏れが構造的に起きない。
//
// 保護・検証に落ちて削除しなかった対象も「中止」として 1 レコード記録する。
// 消さなかった事実を後から追えるようにするためであり、削除できた対象だけを
// 記録すると「対象になったが記録が無い」行が生まれ、監査ログの欠落と見分けが
// つかなくなる。
func removeTarget(ctx context.Context, lg *audit.Logger, t Target) error {
	start := time.Now()
	err := checkRemovable(t)
	if err == nil {
		if rerr := removeTree(ctx, t.Path); rerr != nil {
			err = fmt.Errorf("%s の削除に失敗しました: %w", t.Label, rerr)
		}
	}

	rec := audit.Record{
		Action:     actionClean,
		Runner:     t.Runner,
		Dir:        t.Base,
		Command:    []string{auditRemoveLabel, t.Path},
		DurationMS: time.Since(start).Milliseconds(),
	}
	if err != nil {
		rec.ExitCode = 1
		rec.Error = err.Error()
	}
	lg.Report(rec)

	return err
}

// checkRemovable は t を削除してよいかを判定する。中止する理由があればエラーを返す。
func checkRemovable(t Target) error {
	if t.Protected != "" {
		return fmt.Errorf("%s の削除を中止しました: %s", t.Label, t.Protected)
	}
	if err := ValidatePath(t.Base, t.Path); err != nil {
		return fmt.Errorf("%s の削除を中止しました: %w", t.Label, err)
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
