package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"

	"github.com/ousiassllc/gsr-helper/internal/setup/valid"
)

// 入力検証（FR-36）。規則そのものは internal/setup/valid が持ち、ここはそれを
// 設定編集の言葉（どの欄の値か）で包む層である。同じ規則を 2 つ持つと、追加の
// フォームと設定編集のフォームで通る値が食い違う。
//
// **work dir の検証はここには無い。** Config タブは work dir を編集できない
// （変更には再登録が要るため行ごと選択不可にしてある）ので、書き込み可否と
// 残容量を見る検証には呼び出し元が無い。呼び出し元の無い公開 API は置かない
// （docs/components/overview.md）。

// ValidateAbsPath は絶対パスであることと .. を含まないことを確かめ、
// Clean 済みのパスを返す（FR-36）。
//
// field には呼び出し側の欄の名前（「監査ログ」など）を渡す。エラー文言に載る
// のがその欄の名前でないと、利用者はどの入力を直せばよいか分からない。
// valid.Dir が既に field を文言の先頭へ付けるため、ここでは包み直さない。
// 包むと「監査ログ: 監査ログ: 絶対パスを指定してください」と二重になる。
func ValidateAbsPath(field, path string) (string, error) {
	return valid.Dir(field, path)
}

// CustomLabels は GitHub が自動で付ける予約ラベルを除いた並びを返す（FR-35）。
//
// ラベルの一覧 API は self-hosted / Linux / X64 を含む全量を返すが、これらは
// 読み取り専用で ValidateLabels が拒否する。除かずにフォームの初期値にすると、
// 開いた時点で自分の検証に落ちて確定できないフォームになる。置換 API へ渡す値
// からも除く（GitHub が読み取り専用のラベルを付け直す）。
func CustomLabels(labels []string) []string {
	out := make([]string, 0, len(labels))
	for _, l := range labels {
		if valid.IsReserved(l) {
			continue
		}
		out = append(out, l)
	}
	return out
}

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
