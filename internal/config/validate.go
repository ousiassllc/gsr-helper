package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"

	"github.com/ousiassllc/gsr-helper/internal/setup/valid"
)

// 入力検証のエラー（FR-36）。呼び出し側は errors.Is で判定する。
//
// ラベルと runner 名の検証は internal/setup/valid が持つものをそのまま使う。
// 同じ規則を 2 つ持つと、追加のフォームと設定編集のフォームで通る値が食い違う。
// ここに足すのは work dir の検証だけである。
var (
	// ErrWorkDirNotWritable は work dir に書き込めない場合のエラー。
	ErrWorkDirNotWritable = errors.New("work dir に書き込めません")
	// ErrWorkDirLowSpace は work dir の残容量が足りない場合のエラー。
	ErrWorkDirLowSpace = errors.New("work dir の残容量が足りません")
)

// MinWorkDirFreeBytes は work dir に求める残容量の既定値。
//
// 1 GiB とするのは、checkout とビルド成果物を置く場所として最低限であり、
// これを下回る状態で runner を動かすとジョブが途中で失敗するためである。
// 呼び出し側は ValidateWorkDir の引数で上書きできる。
const MinWorkDirFreeBytes int64 = 1 << 30

// WorkDirInfo は work dir の検証に要するファイルシステム側の事実。
type WorkDirInfo struct {
	// Writable は work dir（無ければ作成先の親）に書き込めるか。
	Writable bool
	// AvailBytes は一般ユーザーが使える残容量。
	AvailBytes int64
}

// WorkDirProbe は work dir の状態を調べる関数。
//
// 関数として受け取るのは、ValidateWorkDir を純粋関数に保つためである
// （docs/components/overview.md は Validate* を純粋関数と定めている）。
// 本番の呼び出し側は ProbeWorkDir を渡す。
type WorkDirProbe func(dir string) (WorkDirInfo, error)

// ValidateLabels はラベルを検証し、整えた並びを返す（FR-36）。
//
// 文字種・重複・予約ラベル（self-hosted / linux / x64）の判定は
// internal/setup/valid に委ねる。前後の空白の除去と重複の除去も済んだ値が返る。
func ValidateLabels(labels []string) ([]string, error) {
	out, err := valid.Labels(labels)
	if err != nil {
		return nil, fmt.Errorf("ラベル: %w", err)
	}
	return out, nil
}

// ValidateRunnerName は runner 名を検証する（FR-36）。
//
// existing には同じホスト内の既存の runner 名を渡す。ホスト内の重複を弾くのは、
// ディレクトリ名とユニット名が名前から決まるためである。
func ValidateRunnerName(name string, existing []string) error {
	if err := valid.Name(name, existing); err != nil {
		return fmt.Errorf("runner 名: %w", err)
	}
	return nil
}

// ValidateWorkDir は work dir を検証し、整えた絶対パスを返す（FR-36）。
//
// 絶対パスであることと .. を含まないことは internal/setup/valid に委ね、
// 書き込み可否と残容量を probe の結果で判定する。minFree が 0 以下なら
// MinWorkDirFreeBytes を使う。
//
// probe が nil の場合は書き込み可否と残容量を見ない。パスの形だけを確かめたい
// 呼び出し（入力中の逐次検証など）で、打鍵のたびに statfs を呼ばないためである。
func ValidateWorkDir(path string, minFree int64, probe WorkDirProbe) (string, error) {
	clean, err := valid.Dir("work dir", path)
	if err != nil {
		return "", fmt.Errorf("work dir: %w", err)
	}
	if probe == nil {
		return clean, nil
	}
	if minFree <= 0 {
		minFree = MinWorkDirFreeBytes
	}

	info, err := probe(clean)
	if err != nil {
		return "", fmt.Errorf("work dir の確認に失敗しました: %w", err)
	}
	if !info.Writable {
		return "", fmt.Errorf("%s: %w", clean, ErrWorkDirNotWritable)
	}
	if info.AvailBytes < minFree {
		return "", fmt.Errorf(
			"%s: %w（残り %d バイト、必要 %d バイト）",
			clean, ErrWorkDirLowSpace, info.AvailBytes, minFree,
		)
	}
	return clean, nil
}

// ErrHookNotFound は job hook のスクリプトが見つからない場合のエラー。
var ErrHookNotFound = errors.New("job hook のスクリプトが見つかりません")

// ErrHookNotExecutable は job hook のスクリプトに実行権が無い場合のエラー。
var ErrHookNotExecutable = errors.New("job hook のスクリプトに実行権がありません")

// HookStat は job hook のスクリプトの状態を調べる関数。
//
// 関数で受け取るのは ValidateHookPath を純粋関数に保つためである
// （Validate* は純粋関数、という components/overview.md の約束）。
// 本番の呼び出し側は StatHook を渡す。
type HookStat func(path string) (fs.FileMode, error)

// ValidateHookPath は job hooks のスクリプトパスを検証する。
//
// **このパスは runner がジョブごとにシェルで実行する**
// （docs/architecture/security.md「入力を検証してから渡す」）。書き換えられる
// 値がそのまま実行に繋がるため、絶対パスであること・存在すること・実行できる
// ことを書き込む前に確かめる。
//
// 空文字は「設定しない」を表すので通す。stat が nil なら存在と実行権を見ない
// （入力中の逐次検証で打鍵のたびに stat を呼ばないため）。
func ValidateHookPath(path string, stat HookStat) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", nil
	}

	clean, err := valid.Dir("job hook", path)
	if err != nil {
		return "", fmt.Errorf("job hook: %w", err)
	}
	if stat == nil {
		return clean, nil
	}

	mode, err := stat(clean)
	if err != nil {
		return "", fmt.Errorf("%s: %w", clean, ErrHookNotFound)
	}
	if !mode.IsRegular() {
		return "", fmt.Errorf("%s: %w", clean, ErrHookNotFound)
	}
	if mode.Perm()&0o111 == 0 {
		return "", fmt.Errorf("%s: %w", clean, ErrHookNotExecutable)
	}
	return clean, nil
}

// StatHook は job hook のスクリプトの状態を実際に調べる。ValidateHookPath へ渡す
// 既定の HookStat である。
func StatHook(path string) (fs.FileMode, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return 0, fmt.Errorf("%s の状態の取得に失敗しました: %w", path, err)
	}
	return fi.Mode(), nil
}
