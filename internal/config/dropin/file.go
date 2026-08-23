package dropin

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/ousiassllc/gsr-helper/internal/config/fileio"
)

const (
	// DefaultRoot は systemd のユニット設定を置く既定のディレクトリ。
	//
	// /etc/systemd/system を使うのは、ここが管理者による上書き用の場所であり、
	// パッケージ側（/usr/lib/systemd/system）と衝突しないためである。
	DefaultRoot = "/etc/systemd/system"

	// FileName は drop-in のファイル名。
	FileName = "override.conf"

	// DirSuffix はユニット名に付ける drop-in ディレクトリの接尾辞。
	DirSuffix = ".d"

	// MaxSize は drop-in として読み込むサイズの上限。
	MaxSize int64 = 1 << 20

	// DirMode は drop-in ディレクトリを新規に作るときのパーミッション。
	// systemd が読めるよう、他者にも読み取りと実行を許す。
	DirMode fs.FileMode = 0o755

	// FileMode は drop-in を新規に作るときのパーミッション。
	// 秘密は書かない前提の設定ファイルであり、systemd が読める必要がある。
	FileMode fs.FileMode = 0o644
)

// ErrBadUnit は drop-in を置けないユニット名を渡された場合のエラー。
var ErrBadUnit = errors.New("systemd ユニット名として扱えません")

// Dir は unit の drop-in ディレクトリ（<root>/<unit>.d）を返す。
// root が空なら DefaultRoot を使う。
func Dir(root, unit string) (string, error) {
	if err := checkUnit(unit); err != nil {
		return "", err
	}
	if root == "" {
		root = DefaultRoot
	}
	return filepath.Join(root, unit+DirSuffix), nil
}

// Path は unit の drop-in ファイル（<root>/<unit>.d/override.conf）を返す。
//
// root を引数に取るのは、既定の /etc/systemd/system へ書くテストが書けないため
// である。本番の呼び出し側は空文字を渡して DefaultRoot を使う。
func Path(root, unit string) (string, error) {
	dir, err := Dir(root, unit)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, FileName), nil
}

// checkUnit はユニット名がパスの一部として安全かを確かめる。
//
// ユニット名は systemd のユニット一覧（内部の runner/systemd が読む）から来る
// 値だが、そこは runner ディレクトリの .service ファイルの中身に由来する。
// つまり runner 実行ユーザーが書き換えられる文字列であり、"../../etc" のような
// 値が来れば root で動く本ツールが意図しない場所へ drop-in を書いてしまう。
func checkUnit(unit string) error {
	switch {
	case unit == "":
		return fmt.Errorf("空のユニット名: %w", ErrBadUnit)
	case strings.ContainsAny(unit, "/\\"):
		return fmt.Errorf("%q はパス区切りを含みます: %w", unit, ErrBadUnit)
	case unit == "." || unit == "..":
		return fmt.Errorf("%q: %w", unit, ErrBadUnit)
	case strings.ContainsRune(unit, 0):
		return fmt.Errorf("ユニット名に NUL が含まれます: %w", ErrBadUnit)
	default:
		return nil
	}
}

// Load は path の drop-in を読む。ファイルが無ければ空の DropIn を返す。
//
// 「まだ drop-in が無い」は普通の状態であり、異常として扱うと編集を始められない。
func Load(path string) (DropIn, error) {
	s, err := ReadRaw(path)
	if err != nil {
		return DropIn{Directives: nil}, err
	}
	return Parse(s), nil
}

// ReadRaw は path の drop-in を書かれているままの文字列で読む。
// ファイルが無ければ空文字を返す。
//
// **差分の before に使う。** Parse はコメント・[Unit]・[Install]・未知の
// セクションを捨て、Render は [Service] だけを書き出す。Render の結果を before に
// すると、手書きの override.conf を編集したときに消える行が差分に 1 行も出ず、
// 利用者は失われることを知らないまま承認してしまう（FR-37）。
func ReadRaw(path string) (string, error) {
	b, err := fileio.Read(path, MaxSize)
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("drop-in の読み込みに失敗しました: %w", err)
	}
	return string(b), nil
}

// Save は d を path へ書き出す。親ディレクトリが無ければ作る。
//
// 書き込みは fileio.Write に委ねるので、既存ファイルがあれば所有者と
// パーミッションを引き継ぎ、失敗しても元のファイルは壊れない。
//
// 反映には systemctl daemon-reload が要る（FR-35 の反映方法の表）。この
// パッケージはファイルを置くだけで、再読み込みは呼び出し側の責務である
// （ドメイン層は外部コマンドを直接起動しない）。
func Save(d DropIn, path string) error {
	if err := os.MkdirAll(filepath.Dir(path), DirMode); err != nil {
		return fmt.Errorf("drop-in ディレクトリの作成に失敗しました: %w", err)
	}
	if err := fileio.Write(path, []byte(d.Render()), FileMode); err != nil {
		return fmt.Errorf("drop-in の書き込みに失敗しました: %w", err)
	}
	return nil
}
