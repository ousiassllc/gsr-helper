package discovery

import (
	"context"
	"testing"
	"time"

	"github.com/ousiassllc/gsr-helper/internal/exec"
	"github.com/ousiassllc/gsr-helper/internal/runner"
)

// Start の Cmd を実行して Msg を取り出す。
func runStart(seq int, opts runner.Options) Msg {
	return Start(seq, opts)().(Msg)
}

// 成功した検出の通し番号（Seq）は呼び出し側が渡した値のまま Msg に伝わる。
//
// 古い周期の結果で新しい結果を上書きしないための番号なので（Msg.Seq の doc）、
// Start を経由しても値が変わらないことを固定する。
func TestStartPropagatesSeqOnSuccess(t *testing.T) {
	const seq = 7
	msg := runStart(seq, runner.Options{Exec: exec.NewFake()})

	if msg.Seq != seq {
		t.Errorf("Msg.Seq = %d, want %d", msg.Seq, seq)
	}
	if msg.Err != nil {
		t.Errorf("Msg.Err = %v, want nil（期限内に終わる検出）", msg.Err)
	}
}

// 検出が期限内に終わらなかった場合、Msg.Err は非 nil になる。
//
// Start が課す期限は Budget（15 秒）で固定されており、実際に期限切れにして
// 確かめると通常のテスト実行を 15 秒引き延ばす。Start はこの判定を timeoutErr へ
// 委ねているだけなので（Start の doc）、期限切れの分岐そのものは timeoutErr を
// 直に呼んで検証する。
func TestTimeoutErrReturnsErrorWhenContextExpired(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 0)
	defer cancel()
	<-ctx.Done()

	if err := timeoutErr(ctx, time.Second); err == nil {
		t.Error("期限切れの ctx で timeoutErr が nil を返した")
	}
}

// 期限内に終わっていれば timeoutErr は nil を返す。Start が成功した周期でも
// Msg.Err を立てないことの裏付け。
func TestTimeoutErrReturnsNilWhenContextNotExpired(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	if err := timeoutErr(ctx, time.Minute); err != nil {
		t.Errorf("timeoutErr = %v, want nil", err)
	}
}

// systemctl が無い環境（systemd が偽）では nil を返す。
//
// runner.Discover は Executor が nil のとき systemd を参照しない（Exec の doc）。
func TestExecReturnsNilWithoutSystemd(t *testing.T) {
	if got := Exec(false, exec.NewFake()); got != nil {
		t.Errorf("Exec(false, ...) = %v, want nil（systemd 不在の縮退）", got)
	}
}

// systemd があれば渡された Executor をそのまま返す。
func TestExecReturnsExecutorWithSystemd(t *testing.T) {
	fake := exec.NewFake()
	if got := Exec(true, fake); got != fake {
		t.Errorf("Exec(true, ex) = %v, want ex", got)
	}
}
