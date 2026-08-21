package appconfig

import (
	"context"
	"strings"
	"time"

	"github.com/ousiassllc/gsr-helper/internal/exec"
)

// envGHToken は最優先で読むトークンの環境変数。
const envGHToken = "GH_TOKEN"

// TokenFunc はトークンを取得できるかを判定する関数。
//
// 取得した値そのものは返さない。呼び出し側に値を渡さないことで、判定のためだけに
// 取り出したトークンが Caps や画面に載る経路を作らない（security.md「保持と出力」）。
type TokenFunc func(ctx context.Context, ex exec.Executor) bool

// hasTokenDefault は Options.HasToken が未指定のときに使う暫定のトークン判定。
//
// 優先順は security.md「取得の優先順」のとおり:
//  1. 環境変数 GH_TOKEN
//  2. sudo -u $SUDO_USER gh auth token（SUDO_USER があり実効 UID が 0 のとき）
//  3. gh auth token
//
// 2 番目があるのは、sudo で起動した場合の gh auth token が root の gh 設定を
// 参照してしまい、通常ユーザーで済ませた認証が読めないためである。
//
// トークンの取得は本来 internal/gh の責務なので、そちらの実装後は
// Options.HasToken = gh.HasToken を渡し、本ファイルを削除する。
func hasTokenDefault(ctx context.Context, ex exec.Executor, p probes, timeout time.Duration) bool {
	// 環境変数で分かるならコマンドを一切発行しない。
	if strings.TrimSpace(p.getenv(envGHToken)) != "" {
		return true
	}
	// gh が無いなら失敗すると分かっている実行をしない（無駄な監査ログを残さない）。
	if !available(p, "gh") {
		return false
	}

	// sudo -u の引数になるため文字種を検証済みの値を使う。不正ならこの経路を飛ばす。
	if su := sudoUserFromEnv(p.getenv); p.geteuid() == 0 && su != "" {
		if runProbe(ctx, ex, "caps.token", timeout, "sudo", "-u", su, "gh", "auth", "token") {
			return true
		}
	}
	return runProbe(ctx, ex, "caps.token", timeout, "gh", "auth", "token")
}
