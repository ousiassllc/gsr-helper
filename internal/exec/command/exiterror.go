package command

import (
	"fmt"
	"strings"
)

// ExitError はコマンドが非ゼロで終了したことを表す。
// Args / Stderr はマスク済みで、そのまま画面やログに出してよい。
type ExitError struct {
	Name string
	Args []string
	Code int
	// Stderr はマスク済みの標準エラー出力の末尾（最大 maxStderrExcerptBytes）。
	// 切り落とした場合は先頭に elisionPrefix が付く。
	Stderr string
	// Err は基になる *osexec.ExitError。errors.As で到達できるようにするため保持する。
	Err error
}

// Summary は標準エラー出力を含まない 1 行の要約を返す。
//
// 監査ログに載せるのはこちらである。stderr には値一致マスクしか効かず、秘密情報の
// 提供元に無い資格情報を子がエコーすればそのまま残るため、ディスクに残る経路には
// 流さない（docs/architecture/security.md は sudo -l -U について実行の事実と
// 終了コードだけを記録するよう求めている）。
func (e *ExitError) Summary() string {
	cmdline := strings.Join(append([]string{e.Name}, e.Args...), " ")
	return fmt.Sprintf("%s が終了コード %d で失敗しました", cmdline, e.Code)
}

// Error はコマンド行と終了コード、あれば標準エラー出力を含めた文を返す。
//
// Summary と違い stderr を含めるのは、利用者が原因を読む唯一の手掛かりであり、
// UI に出す文としては欠かせないためである。
func (e *ExitError) Error() string {
	if s := strings.TrimSpace(e.Stderr); s != "" {
		return e.Summary() + ": " + s
	}
	return e.Summary()
}

// Unwrap は基になるエラーを返す。呼び出し側が errors.As で *osexec.ExitError まで
// 到達できるようにするため。osexec.ExitError.Error() は "exit status 3" のみで
// 引数を含まないため、これを露出しても情報漏洩にはならない。
func (e *ExitError) Unwrap() error { return e.Err }
