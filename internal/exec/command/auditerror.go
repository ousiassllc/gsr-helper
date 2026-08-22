package command

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/ousiassllc/gsr-helper/internal/exec/mask"
)

// AuditError は監査ログへの記録が失敗したことを表す。
//
// コマンド自体の失敗（*ExitError など）と別の型にしているのは、Run の返り値に
// 混ぜないためである。記録できなかっただけで成功した操作を失敗として返すと、
// TUI が「実際には動いた操作」を失敗として見せてしまう。
type AuditError struct {
	// Err は基になる書き込みエラー。
	Err error
}

// Error は記録の失敗であってコマンドの失敗ではないと分かる文を返す。
func (e *AuditError) Error() string {
	return fmt.Sprintf("監査ログの記録に失敗しました（コマンド自体の成否とは無関係です）: %v", e.Err)
}

// Unwrap は基になる書き込みエラーを返す。
func (e *AuditError) Unwrap() error { return e.Err }

// auditErrorSink は通知先が未設定のときの書き出し先。
//
// os.Stderr を直接書くとテストから検証できないため変数にしている。
// 差し替えるのはテストだけで、通常の経路では os.Stderr のまま使う。
var auditErrorSink io.Writer = os.Stderr

// reportAuditError は監査記録の失敗を通知する。Run の返り値には載せない。
func (c *Command) reportAuditError(err error) {
	aerr := &AuditError{Err: err}
	if c.auditErr != nil {
		c.auditErr(aerr)
		return
	}
	// 通知先が無いときに黙って捨てると、記録漏れが誰にも気付かれないまま
	// 監査ログだけが欠ける。最後の砦として標準エラー出力へ 1 行だけ出す。
	// TUI アプリが WithAuditErrorFunc を必ず設定すべき理由もこれである。
	_, _ = fmt.Fprintln(auditErrorSink, aerr.Error())
}

// recordedError は監査レコードの error に載せる文を作る。
//
// *ExitError はコマンドの標準エラー出力を含むため Error() ではなく Summary() を
// 使う（stderr をディスクに残さないという方針を型の側で担保する）。それ以外の
// エラーはマスクした文を載せるが、いくらでも長くなり得るので上限で切る。
func recordedError(err error, secrets []string) string {
	msg := err.Error()

	var exitErr *ExitError
	if errors.As(err, &exitErr) {
		msg = exitErr.Summary()
	}
	return truncateTail(mask.String(msg, secrets), maxRecordedErrorBytes)
}
