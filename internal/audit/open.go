package audit

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// dirMode / fileMode は監査ログのパーミッション。
// 監査ログには実行したコマンド全文が残るため、root 以外に読ませない。
const (
	dirMode  os.FileMode = 0o700
	fileMode os.FileMode = 0o600
)

// Open は path に追記する Logger を返す。opts は New と同じ設定を受け付ける。
//
// 可変長の設定を受けるのは、識別子（uid / sudo_user）を呼び出し側が明示できる
// ようにするためである。既定は環境変数の生値だが、cmd は検証済みの SUDO_USER を
// 持っているので、それを渡せる経路が必要になる。
//
// 親ディレクトリが無ければ 0o700 で作る。既存ディレクトリのモードは変更しない
// （/var/log 配下など、本ツールが作っていないディレクトリの権限を勝手に変えない）。
// ファイルは 0o600 で開き、既存ファイルが緩い権限で残っていた場合は Chmod で締める。
//
// 返した Logger は使い終わったら Close すること。
func Open(path string, opts ...Option) (*Logger, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, dirMode); err != nil {
		return nil, fmt.Errorf("%s の作成に失敗しました: %w", dir, err)
	}

	// O_NOFOLLOW で最終要素がシンボリックリンクなら開かずに失敗させる。本ツールは
	// sudo 前提で動くため、親ディレクトリが他ユーザーに書けるときに攻撃者が
	// audit.jsonl を任意ファイルへのリンクとして先置きすると、root が Chmod 0600 と
	// 追記を代行させられる。docs/architecture/security.md が「シンボリックリンク
	// 経由の逸脱」を脅威に挙げているため、ここで断つ。
	//nolint:gosec // path は本ツール自身の設定（audit_log）で与えられる出力先であり、外部入力ではない。
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY|syscall.O_NOFOLLOW, fileMode)
	if err != nil {
		return nil, fmt.Errorf("%s のオープンに失敗しました: %w", path, err)
	}

	// OpenFile のモード引数は新規作成時のみ効くため、既存ファイルは明示的に締める。
	if err := f.Chmod(fileMode); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("%s のパーミッション設定に失敗しました: %w", path, err)
	}

	l := New(f, opts...)
	l.closer = f
	return l, nil
}
