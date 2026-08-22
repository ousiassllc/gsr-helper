package appconfig

// 下位パッケージの入口をここで再公開する。
//
// 設定の配置先（confpath）と能力判定（hostcaps）は責務として分離しているが、
// 呼び出し側から見ればどちらも「gsr-helper 自身の設定」の一部である。ここで
// 別名を与えておくことで、UI や cmd が下位パッケージを個別に import せずに
// 済み、内部の分割を後から変えても呼び出し側に波及しない。

import (
	"context"

	"github.com/ousiassllc/gsr-helper/internal/appconfig/confpath"
	"github.com/ousiassllc/gsr-helper/internal/appconfig/hostcaps"
	"github.com/ousiassllc/gsr-helper/internal/exec"
)

// Caps は起動時に 1 回判定する能力。詳細は hostcaps.Caps を参照。
type Caps = hostcaps.Caps

// Options は Detect の設定。詳細は hostcaps.Options を参照。
type Options = hostcaps.Options

// TokenFunc はトークンを取得できるかを判定する関数。詳細は hostcaps.TokenFunc を参照。
type TokenFunc = hostcaps.TokenFunc

// Detect は能力を判定する。判定に失敗した能力は「無い」として扱う。
func Detect(ctx context.Context, ex exec.Executor, opts Options) Caps {
	return hostcaps.Detect(ctx, ex, opts)
}

// DefaultPath は設定ファイルの既定の配置先を返す。
func DefaultPath() (string, error) { return confpath.Default() }

// SudoUser は検証済みの SUDO_USER を返す。文字種が不正なら空文字を返す。
//
// Caps を得る前（監査ログを開く時点。Detect は Executor を必要とし、Executor は
// 監査ログを必要とする）にも要るため関数として公開する。Caps を持っているなら
// Caps.SudoUser を使うこと。
func SudoUser() string { return confpath.SudoUser() }
