package disk

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"

	"github.com/ousiassllc/gsr-helper/internal/runner"
)

// 集計対象になるディレクトリ名。ValidatePath が許可するサブツリーと同じものを
// 見るため、リテラルで持つ。workFolder を既定から変えた runner は削除が許可された
// サブツリーの外になるため、集計対象にもしない。
const (
	workDirName = "_work"
	diagDirName = "_diag"
	toolDirName = "_tool"
	tempDirName = "_temp"
)

// busyReason はジョブ実行中で削除できない場合の理由（FR-31）。
//
// 表示幅 22 セルに収まる長さにしてある。理由は Disk タブの PATH 列に載るが、この列は
// 一覧の最終列であり、organism/table が bubbles/table のセル余白の分だけ最終列を 1 セル
// 狭めるため、使えるのは token.DiskColumns の 26 セルではなく 25 セルである。これより
// 長いと末尾が中略され、FR-31 が求める「理由の表示」が「ジョブ実行中のため削除不…」に
// なって読めない（docs/ui/screens.md の Disk タブのモックも同じ文言を使う）。
const busyReason = "ジョブ実行中で削除不可"

// Scan は r の使用量を対象ごとに並行して集計し、判明した順に out へ送る（FR-27 / FR-28）。
//
// 対象ごとに goroutine を分けるのは、巨大な _work/<リポジトリ> 1 件の走査が
// 他の対象の表示を待たせないようにするためである（UI は判明した行から順に埋める）。
//
// out は閉じない。呼び出し側が複数 runner の Scan を 1 本の channel に集約するため、
// 閉じる責務は集約する側にある。Scan はすべての対象を送り終えてから返るので、
// 呼び出し側は WaitGroup で待ってから閉じられる。
func Scan(ctx context.Context, r runner.Runner, out chan<- Usage) {
	var wg sync.WaitGroup
	for _, u := range scanTargets(r) {
		wg.Add(1)
		go func() {
			defer wg.Done()

			u.Bytes, u.Files, u.Err = walkTree(ctx, u.Path)
			select {
			case out <- u:
			case <-ctx.Done():
			}
		}()
	}
	wg.Wait()
}

// scanTargets は r の集計対象を列挙する。
//
// _work が無い・読めない runner はエラー行を並べずに対象 0 件として扱う。
// 起動直後でジョブを 1 度も実行していない runner は _work を持たないため、
// 異常として見せると正常な状態がエラーで埋まる。
func scanTargets(r runner.Runner) []Usage {
	workDir := filepath.Join(r.Dir, workDirName)
	var targets []Usage

	entries, err := os.ReadDir(workDir)
	if err == nil {
		for _, e := range entries {
			kind, ok := workEntryKind(e)
			if !ok {
				continue
			}
			targets = append(targets, newTarget(r, kind,
				filepath.Join(workDir, e.Name()), workDirName+"/"+e.Name()))
		}
	}

	// _diag はジョブ実行中でも削除できる（security.md §3 がブロック対象としているのは
	// _work 配下だけで、_diag は実行中のジョブが書き込むログの置き場ではない）。
	diagDir := filepath.Join(r.Dir, diagDirName)
	if fi, serr := os.Stat(diagDir); serr == nil && fi.IsDir() {
		targets = append(targets, newTarget(r, KindDiag, diagDir, diagDirName))
	}
	return targets
}

// workEntryKind は _work 直下のエントリの種別を返す。集計対象でなければ ok が false。
func workEntryKind(e fs.DirEntry) (Kind, bool) {
	switch e.Name() {
	case toolDirName:
		return KindTool, true
	case tempDirName:
		return KindTemp, true
	default:
		// リポジトリごとの作業ディレクトリ。ディレクトリ以外（_work 直下に
		// 置かれる作業ファイル）は内訳の単位にならないため対象にしない。
		return KindWork, e.IsDir()
	}
}

// newTarget は 1 対象の Usage の雛形を作る。rel は表示用の runner からの相対パス。
func newTarget(r runner.Runner, kind Kind, path, rel string) Usage {
	removable, reason := canRemove(r, kind)
	return Usage{
		Kind:      kind,
		Runner:    r.Name(),
		Base:      r.Dir,
		Path:      path,
		Label:     r.Name() + " / " + rel,
		Bytes:     0,
		Files:     0,
		Removable: removable,
		Reason:    reason,
		Err:       nil,
	}
}

// canRemove は対象を削除対象として選べるかと、選べない理由を返す（FR-31）。
func canRemove(r runner.Runner, kind Kind) (bool, string) {
	if kind != KindDiag && r.Busy() {
		return false, busyReason
	}
	return true, ""
}

// walkTree は root 配下のサイズとファイル数を数える。
//
// 外部の du を使わないのは、進捗を出せるようにするためと、外部プロセスの出力解析を
// 挟まないためである（docs/api/external-interfaces.md）。
//
// シンボリックリンクは辿らない（filepath.WalkDir の既定）。リンク自体は 1 ファイルと
// 数えるがサイズは 0 とする。リンク先の実体は、それが集計対象の中にあれば別途数えられ、
// 外にあれば削除しても解放されないためである。
// 読めないエントリはスキップして走査を続ける。権限不足のディレクトリが 1 つあっても
// 残りの内訳は出せるようにするためで、root 自体が読めない場合だけエラーにする。
func walkTree(ctx context.Context, root string) (bytes, files int64, err error) {
	werr := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if cerr := ctx.Err(); cerr != nil {
			return cerr
		}
		if err != nil {
			if path == root {
				return err
			}
			return nil // 読めないエントリは飛ばす（ディレクトリなら配下ごと飛ぶ）
		}
		if d.IsDir() {
			return nil
		}
		info, ierr := d.Info()
		if ierr != nil {
			return nil // 走査中に消えたファイル。数えずに続ける
		}
		files++
		if info.Mode()&fs.ModeSymlink == 0 {
			bytes += info.Size()
		}
		return nil
	})
	if werr != nil {
		return 0, 0, fmt.Errorf("%s の集計に失敗しました: %w", root, werr)
	}
	return bytes, files, nil
}
