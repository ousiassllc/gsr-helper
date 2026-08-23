package envfile

import (
	"errors"
	"fmt"
	"io/fs"
	"strings"

	"github.com/ousiassllc/gsr-helper/internal/config/fileio"
)

// ErrMultiLine は .path に改行を含む値を書き出そうとした場合のエラー。
var ErrMultiLine = errors.New(".path は 1 行でなければなりません")

// PathFile は .path の中身。1 行のパス文字列だけを持つ。
//
// .env と違って構造が無いため、行の保持という仕組みは要らない。
type PathFile struct {
	// Value は PATH に相当する 1 行。末尾の改行は含まない。
	Value string
}

// LoadPath は path の .path を読む。ファイルが無ければ空の PathFile を返す。
//
// 末尾の改行は 1 つだけ落とす。落としきらないのは、2 行以上ある .path が
// 「壊れている」という事実を握り潰さないためである。その場合 SavePath が
// ErrMultiLine で書き込みを拒む。
func LoadPath(path string) (PathFile, error) {
	b, err := fileio.Read(path, MaxSize)
	if errors.Is(err, fs.ErrNotExist) {
		return PathFile{Value: ""}, nil
	}
	if err != nil {
		return PathFile{Value: ""}, fmt.Errorf(".path の読み込みに失敗しました: %w", err)
	}
	return PathFile{Value: trimEOL(string(b))}, nil
}

// SavePath は p を path へ書き出す。末尾に改行をちょうど 1 つ付ける。
// Value に改行が含まれていれば ErrMultiLine を返し、何も書き込まない。
func SavePath(p PathFile, path string) error {
	if strings.ContainsAny(p.Value, "\r\n") {
		return fmt.Errorf("%s: %w", path, ErrMultiLine)
	}
	if err := fileio.Write(path, []byte(p.Value+"\n"), FileMode); err != nil {
		return fmt.Errorf(".path の書き込みに失敗しました: %w", err)
	}
	return nil
}

// trimEOL は末尾の改行（CRLF / LF）を 1 つだけ落とす。
func trimEOL(s string) string {
	s = strings.TrimSuffix(s, "\n")
	return strings.TrimSuffix(s, "\r")
}
