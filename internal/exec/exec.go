// Package exec は外部プロセス実行の唯一の経路。
//
// ドメイン層が os/exec を直接使わないという規則（docs/components/overview.md の
// 「依存の規則」）の受け皿であり、タイムアウト・監査ログ・トークンマスクの
// 適用漏れを構造的に防ぐ。呼び出し側がこれらを忘れられる余地を作らない。
//
// 実装は 2 つ。Command が実プロセスを起動し、Fake がテストで発行コマンドを記録する。
package exec

import (
	"context"
	"fmt"
	osexec "os/exec"
)

// Executor は外部プロセス実行の interface。
//
// 1 回の実行ごとに変わるパラメータ（作業ディレクトリ・環境変数・監査ログ用の
// メタ情報）は ctx に載せて渡す。詳細は Options を参照。
type Executor interface {
	Run(ctx context.Context, name string, args ...string) (Result, error)
}

// 実装がこの interface を満たしていることをコンパイル時に確かめる。
var (
	_ Executor = (*Command)(nil)
	_ Executor = (*Fake)(nil)
)

// Result は 1 回の実行結果。
//
// Stdout / Stderr はマスクしない。UI は実際の出力を見る必要があるためである。
// マスクをかけるのは記録・表示として残る経路、すなわち監査ログとエラー文だけ。
type Result struct {
	Stdout, Stderr []byte
	// ExitCode は終了コード。プロセスを起動できなかった場合は -1。
	ExitCode int
}

// LookPath は os/exec.LookPath のラッパー。
//
// ドメイン層が os/exec を import せずにコマンドの有無を判定できるようにするために置いている。
// プロセスを起動しないため監査ログには記録しない。
func LookPath(name string) (string, error) {
	path, err := osexec.LookPath(name)
	if err != nil {
		return "", fmt.Errorf("%s が見つかりません: %w", name, err)
	}
	return path, nil
}
