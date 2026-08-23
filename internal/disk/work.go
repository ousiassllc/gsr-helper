package disk

import (
	"context"
	"os"
	"path/filepath"

	"github.com/ousiassllc/gsr-helper/internal/runner"
)

// WorkUsage は runner 1 台の _work ディレクトリ全体の使用量を返す（FR-27 / Issue #73）。
//
// **Scan とは用途が違う。** Scan は削除の単位（_work/<リポジトリ> / _tool / _temp /
// _diag）ごとに内訳を出し、判明した順に送る。Disk タブが並べて選ばせるためである。
// こちらは Runners タブの `_WORK` 列と runner 詳細画面の work 行に出す**合計 1 つ**
// だけを返す。内訳を要求しないぶん channel も goroutine も要らず、呼び出し側は
// 「1 台につき 1 つの値」として扱える。
//
// _work が無い runner は 0 とエラー無しを返す。ジョブを 1 度も実行していない runner は
// _work を持たないので、それを異常として見せると起動直後の一覧がエラーで埋まる
// （scanTargets が対象 0 件として扱うのと同じ理由）。
//
// 読めないエントリは飛ばして走査を続ける（walkTree の契約）。_work 自体が読めない
// 場合だけエラーを返し、呼び出し側はその runner を `-` に縮退させる。
//
// **ctx で必ず打ち切れること。** 巨大な _work では分単位かかりうるため、呼び出し側は
// 期限を張る。打ち切られた場合はエラーを返し、途中までの値を「集計できた値」として
// 見せない（途中経過を出すと、実際より小さい使用量を確定値として読ませる）。
func WorkUsage(ctx context.Context, r runner.Runner) (int64, error) {
	work := filepath.Join(r.Dir, workDirName)

	fi, err := os.Stat(work)
	if err != nil || !fi.IsDir() {
		// 無い・読めないは「0 バイト」として扱う。上の doc を参照。
		return 0, nil
	}

	bytes, _, err := walkTree(ctx, work)
	if err != nil {
		return 0, err
	}
	return bytes, nil
}
