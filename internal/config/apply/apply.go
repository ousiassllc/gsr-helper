// Package apply は設定を書き込んだあとの反映方法を表し、実行する（FR-39）。
//
// 反映のコストは設定項目ごとに違う（機能要件の「反映方法」の表）。.env / .path は
// runner の再起動、drop-in は daemon-reload を足した再起動、ラベルと runner group は
// GitHub API で即時に反映され再起動が要らない。ここが受け持つのは前の 2 つである。
//
// **internal/svc は変更せず組み合わせるだけである**（Issue #12 の影響範囲）。
// 「ドレイン再起動」に相当する操作は svc に無い。svc.Drain は実行中ジョブの完了を
// 待って停止するところまでなので、そのあと svc.Start を呼んで組み立てる。
//
// ドメイン層なので UI を知らない。待機の表示に要る経過情報は Progress で渡す。
package apply

import (
	"context"
	"fmt"

	"github.com/ousiassllc/gsr-helper/internal/exec"
	"github.com/ousiassllc/gsr-helper/internal/runner"
	"github.com/ousiassllc/gsr-helper/internal/svc"
)

// Method は書き込んだ設定の反映方法。
//
// ゼロ値を Drain にしているのは、画面仕様が「ドレイン再起動」を既定の提案と
// 定めているためである（FR-39）。既定を明示的に選び直さないと危険側に倒れる
// 並びにはしない。
type Method int

// Method の取り得る値。選択肢の並び順と一致する。
const (
	// Drain はドレイン再起動。実行中ジョブの完了を待って再起動する（既定）。
	Drain Method = iota
	// Force は強制再起動。実行中のジョブは中断される。
	Force
	// None は反映しない。次回起動時に有効になる。
	None
)

// labels は選択肢に出す文言。画面仕様「反映方法を選んでください」のモックに合わせる。
var labels = [...]string{
	Drain: "ドレイン再起動（実行中ジョブの完了を待って再起動）",
	Force: "強制再起動（⚠ 実行中のジョブは中断されます）",
	None:  "反映しない（次回起動時に有効）",
}

// Label は選択肢に出す文言を返す。
func (m Method) Label() string {
	if m < 0 || int(m) >= len(labels) {
		return labels[Drain]
	}
	return labels[m]
}

// Methods は選択肢を既定（ドレイン再起動）を先頭にした順で返す。
func Methods() []Method { return []Method{Drain, Force, None} }

// DrainFunc はドレイン停止を行う関数。テストで差し替えるために型で受ける。
type DrainFunc func(ctx context.Context, ex exec.Executor, r runner.Runner, progress func(svc.Progress)) error

// Input は反映の実行に要るもの。
type Input struct {
	// Exec は外部コマンドの実行手段。
	Exec exec.Executor
	// Runner は対象。
	Runner runner.Runner
	// Method は反映方法。
	Method Method
	// Reload は systemctl daemon-reload を先に行うか。drop-in を変えた場合に真にする。
	Reload bool
	// Progress はドレイン待機の経過を受け取る。nil でよい。
	Progress func(svc.Progress)
	// Drain はドレイン停止の実装。nil なら svc.Drain を使う。
	Drain DrainFunc
}

// Run は反映を実行する。
//
// daemon-reload を Method より先に、かつ None のときも行うのは、drop-in を置いた
// だけでは systemd が新しい内容を読まないためである。「反映しない」が意味するのは
// 「稼働中のジョブを止めてまで今すぐ反映はしない」であって、次回起動時にも効かない
// 状態で放置することではない。
func Run(ctx context.Context, in Input) error {
	if in.Reload {
		if err := svc.DaemonReload(ctx, in.Exec); err != nil {
			return fmt.Errorf("daemon-reload に失敗しました: %w", err)
		}
	}

	switch in.Method {
	case None:
		return nil
	case Force:
		if err := svc.Restart(ctx, in.Exec, in.Runner); err != nil {
			return fmt.Errorf("再起動に失敗しました: %w", err)
		}
		return nil
	case Drain:
		return drainRestart(ctx, in)
	default:
		return fmt.Errorf("不明な反映方法です: %d", int(in.Method))
	}
}

// drainRestart は実行中ジョブの完了を待って停止し、起動し直す。
//
// svc.Drain は待機のあと停止までを行う。停止で終わると設定を書いた runner が
// 止まったままになるため、続けて起動する。ctx のキャンセルで中断された場合は
// 起動しない（svc.Drain は中断時に停止処理を行わないので、runner は動き続けている）。
func drainRestart(ctx context.Context, in Input) error {
	drain := in.Drain
	if drain == nil {
		drain = svc.Drain
	}

	if err := drain(ctx, in.Exec, in.Runner, in.Progress); err != nil {
		return fmt.Errorf("ドレイン停止に失敗しました: %w", err)
	}
	if err := svc.Start(ctx, in.Exec, in.Runner); err != nil {
		return fmt.Errorf("再起動に失敗しました: %w", err)
	}
	return nil
}
