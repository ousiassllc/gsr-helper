package disk

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
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
// （scanTargets が対象 0 件として扱うのと同じ理由）。**「無い」以外の理由で読めない
// 場合はエラーを返す**——0 を返すと、集計できていないことが「0 バイト」という確定値
// として一覧に出る。
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
	switch {
	case errors.Is(err, fs.ErrNotExist):
		// **エントリ自体があるかで書き分ける。** リンク切れのシンボリックリンク
		// （別ボリュームへ寄せたが未マウント、という今回まさに想定した失敗形）でも
		// os.Stat は ENOENT を返す。0 を返すと「集計できていないのに 0 バイト」と
		// いう確定値になるので、辿れないことをエラーとして伝える。
		if _, lerr := os.Lstat(work); lerr == nil {
			return 0, fmt.Errorf("%s を辿れません（リンク切れの可能性があります）", work)
		}
		// ジョブを 1 度も実行していない runner。0 バイトとして扱う（上の doc）。
		return 0, nil
	case err != nil:
		// 読めない理由がそれ以外（権限など）なら伝える。0 を返すと、集計できて
		// いないことが「0 バイト」という確定値として一覧に出る。
		return 0, fmt.Errorf("%s の情報取得に失敗しました: %w", work, err)
	case !fi.IsDir():
		return 0, fmt.Errorf("%s はディレクトリではありません", work)
	}

	// **シンボリックリンクは辿る。** `_work` を別ボリュームへ寄せた構成では `_work`
	// 自体がリンクになる。filepath.WalkDir は root を Lstat で見るため、辿らないと
	// リンク 1 件だけを見て 0 バイトを返し、**集計できていないのに 0 バイトという
	// 確定値**を一覧に出す（未集計は `-` に縮退させるのが本来の扱い）。
	//
	// 削除（Apply / removeTree）が辿らないのとは扱いが違ってよい。あちらはリンク先の
	// 実体を消さないための約束で、こちらは読むだけである
	// （security.md「シンボリックリンクの扱い」）。
	//
	// **Scan には同じ手当てが要らない。** あちらの走査対象は `_work` の子（`_work/<repo>`
	// など）なので、リンクは経路の途中にあり OS のパス解決が通す。root がリンクになるのは
	// _work 全体を 1 回で数えるこちらだけである。
	target, err := filepath.EvalSymlinks(work)
	if err != nil {
		return 0, fmt.Errorf("%s の解決に失敗しました: %w", work, err)
	}

	bytes, _, err := walkTree(ctx, target)
	if err != nil {
		return 0, err
	}
	return bytes, nil
}
