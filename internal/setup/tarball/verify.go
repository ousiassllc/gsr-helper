package tarball

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Verify は既存ファイルの SHA-256 を検証する。
//
// Fetch はダウンロードの途中で検証するが、再起動をまたいで残っている tarball や
// 利用者が手で置いたファイルを展開する前にも同じ判定が要る。比較は大文字小文字を
// 区別しない。want が空の場合は ErrNoChecksum を返し、検証したことにはしない。
func Verify(path, want string) error {
	expected := strings.TrimSpace(want)
	if expected == "" {
		return ErrNoChecksum
	}

	got, err := digest(path)
	if err != nil {
		return err
	}
	if !strings.EqualFold(got, expected) {
		return fmt.Errorf("%w: %s（期待 %s / 実際 %s）",
			ErrChecksumMismatch, path, strings.ToLower(expected), got)
	}
	return nil
}

// digest はファイルの SHA-256 を 16 進小文字で返す。
func digest(path string) (string, error) {
	f, err := openForRead(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()

	sum := sha256.New()
	if _, err := io.Copy(sum, f); err != nil {
		return "", fmt.Errorf("%s の読み取りに失敗しました: %w", path, err)
	}
	return hex.EncodeToString(sum.Sum(nil)), nil
}

// openForRead は利用者から渡されたパスを読み取り用に開く。
//
// 対象は「検証・展開したい tarball そのもの」であり、パスを絞り込む基準は
// 呼び出し側にしかない。ここでは正規化だけ行い、開けなければそのまま失敗させる。
func openForRead(path string) (*os.File, error) {
	cleaned := filepath.Clean(path)

	f, err := os.Open(cleaned)
	if err != nil {
		return nil, fmt.Errorf("%s を開けませんでした: %w", path, err)
	}
	return f, nil
}
