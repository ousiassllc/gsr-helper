package envfile

import (
	"errors"
	"fmt"
	"io/fs"

	"github.com/ousiassllc/gsr-helper/internal/config/fileio"
)

const (
	// MaxSize は .env / .path として読み込むサイズの上限。
	//
	// KEY=VALUE の羅列に 1 MiB は十分すぎるが、リンクの張り替えや壊れた巨大
	// ファイルでメモリを食い潰さないための歯止めとして置く。
	MaxSize int64 = 1 << 20

	// FileMode は .env / .path を新規に作るときのパーミッション。
	//
	// .env にはプロキシの認証情報が書かれうるため、既定は所有者のみとする。
	// 既存ファイルがある場合はそちらのパーミッションを引き継ぐ（fileio.Write）。
	FileMode fs.FileMode = 0o600
)

// Load は path の .env を読む。ファイルが無ければ空の File を返す。
//
// 「まだ .env が無い」は runner の普通の状態であり、異常として扱うと編集を
// 始められない。読めるが壊れている場合は、解釈できない行として保持する
// （Parse）ので、ここでエラーになるのは読み取り自体の失敗だけである。
func Load(path string) (File, error) {
	b, err := fileio.Read(path, MaxSize)
	if errors.Is(err, fs.ErrNotExist) {
		return File{lines: nil}, nil
	}
	if err != nil {
		return File{lines: nil}, fmt.Errorf(".env の読み込みに失敗しました: %w", err)
	}
	return Parse(b), nil
}

// Save は f を path へ書き出す。既存ファイルの所有者とパーミッションを引き継ぐ。
func Save(f File, path string) error {
	if err := fileio.Write(path, f.Bytes(), FileMode); err != nil {
		return fmt.Errorf(".env の書き込みに失敗しました: %w", err)
	}
	return nil
}
