package gh

import (
	"context"
	"time"

	"github.com/ousiassllc/gsr-helper/internal/exec"
	"github.com/ousiassllc/gsr-helper/internal/gh/ghtoken"
)

// トークンの取得は internal/gh/ghtoken にある。Client に依存しない処理であり、
// 1 ディレクトリ 2000 行の上限を機に分けた（Issue #81）。呼び出し側が
// import 先を切り替えずに済むよう、ここで従来の名前を残す。

// EnvToken は最優先で読むトークンの環境変数。
const EnvToken = ghtoken.EnvToken

// Source はトークンの取得元。
type Source = ghtoken.Source

// Token は既定の取得元でトークンを取得する。
func Token(ctx context.Context, ex exec.Executor) (string, error) {
	return ghtoken.Token(ctx, ex)
}

// HasToken はトークンを取得できるかを返す（能力判定に使う）。
func HasToken(ctx context.Context, ex exec.Executor, timeout time.Duration) bool {
	return ghtoken.HasToken(ctx, ex, timeout)
}
