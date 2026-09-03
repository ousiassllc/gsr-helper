package tarball

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

// fetchTimeout は c が nil のときに使う既定のクライアントの制限時間。
// runner の tarball は 100MB 前後あり、細い回線でも打ち切られない値にしてある。
const fetchTimeout = 30 * time.Minute

// downloadDirMode は保存先ディレクトリのパーミッション。
// 取得物は root で扱うため、他ユーザーからは触らせない。
const downloadDirMode fs.FileMode = 0o700

// downloadFileMode は保存する tarball のパーミッション。
const downloadFileMode fs.FileMode = 0o600

// Fetch は tarball を dir 配下へ取得し、SHA-256 を検証してからパスを返す。
//
// 検証はストリームの途中で計算した値と Info.SHA256 を突き合わせる。失敗した場合は
// 取得したファイルを消してエラーを返す（展開させない。security.md）。
// c が nil なら制限時間付きの既定のクライアントを使う。progress は nil 可。
// ctx のキャンセルは受信の途中で効き、書きかけのファイルは残さない。
func Fetch(ctx context.Context, c *http.Client, in Info, dir string, progress func(Progress)) (string, error) {
	if strings.TrimSpace(in.URL) == "" {
		return "", ErrNoURL
	}
	want := strings.TrimSpace(in.SHA256)
	if want == "" {
		return "", ErrNoChecksum
	}
	name, err := destName(in)
	if err != nil {
		return "", err
	}
	if c == nil {
		c = &http.Client{Transport: nil, CheckRedirect: nil, Jar: nil, Timeout: fetchTimeout}
	}

	if err := os.MkdirAll(dir, downloadDirMode); err != nil {
		return "", fmt.Errorf("保存先 %s の作成に失敗しました: %w", dir, err)
	}
	// 保存先を根に固定する。Filename は destName で 1 要素に絞ってあるが、
	// 根を固定しておけば経路の途中がリンクに差し替えられていても外へ出られない。
	root, err := os.OpenRoot(dir)
	if err != nil {
		return "", fmt.Errorf("保存先 %s を開けませんでした: %w", dir, err)
	}
	defer func() { _ = root.Close() }()

	sum, err := download(ctx, c, in.URL, root, name, progress)
	if err != nil {
		_ = root.Remove(name)
		return "", err
	}
	if !strings.EqualFold(sum, want) {
		_ = root.Remove(name)
		return "", fmt.Errorf("%w（期待 %s / 実際 %s）", ErrChecksumMismatch, strings.ToLower(want), sum)
	}
	return filepath.Join(dir, name), nil
}

// destName は保存するファイル名を 1 パス要素に決める。
//
// Filename は GitHub の応答由来であり、区切りや ".." を含んだ値を素通しすると
// 保存先の外に書き出せてしまう。ここで弾いて 1 要素であることを保証する。
func destName(in Info) (string, error) {
	name := strings.TrimSpace(in.Filename)
	if name == "" {
		u, err := url.Parse(in.URL)
		if err != nil {
			return "", fmt.Errorf("tarball の URL を解釈できません: %s: %w", in.URL, err)
		}
		name = path.Base(u.Path)
	}
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, `/\`) {
		return "", fmt.Errorf("%w: tarball のファイル名が不正です: %q", ErrUnsafePath, name)
	}
	return name, nil
}

// download は本文を root/name へ書き出しながら SHA-256 を計算し、16 進小文字で返す。
func download(
	ctx context.Context,
	c *http.Client,
	rawURL string,
	root *os.Root,
	name string,
	progress func(Progress),
) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", fmt.Errorf("ダウンロード要求の作成に失敗しました: %w", err)
	}

	resp, err := c.Do(req)
	if err != nil {
		return "", fmt.Errorf("tarball のダウンロードに失敗しました: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%w: HTTP %d（%s）", ErrUnexpectedStatus, resp.StatusCode, rawURL)
	}

	f, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, downloadFileMode)
	if err != nil {
		return "", fmt.Errorf("保存先ファイル %s を作成できませんでした: %w", name, err)
	}

	sum := sha256.New()
	copyErr := copyBody(f, sum, resp.Body, max(resp.ContentLength, 0), progress)
	closeErr := f.Close()

	if copyErr != nil {
		return "", copyErr
	}
	if closeErr != nil {
		return "", fmt.Errorf("保存先ファイル %s を閉じられませんでした: %w", name, closeErr)
	}
	return hex.EncodeToString(sum.Sum(nil)), nil
}

// copyBody は body を dst とハッシュへ同時に流し、進捗を報告する。
func copyBody(dst io.Writer, sum hash.Hash, body io.Reader, total int64, progress func(Progress)) error {
	pw := &progressWriter{total: total, done: 0, report: progress}
	if _, err := io.Copy(io.MultiWriter(dst, sum, pw), body); err != nil {
		return fmt.Errorf("tarball の受信に失敗しました: %w", err)
	}
	return nil
}

// progressWriter は書き込んだバイト数を数えて報告するだけの io.Writer。
// io.MultiWriter に混ぜることで、本体の転送処理に進捗の都合を持ち込まずに済む。
type progressWriter struct {
	total  int64
	done   int64
	report func(Progress)
}

// Write は受け取ったバイト数を積み上げ、報告関数があれば呼ぶ。
func (w *progressWriter) Write(p []byte) (int, error) {
	w.done += int64(len(p))
	if w.report != nil {
		w.report(Progress{Downloaded: w.done, Total: w.total})
	}
	return len(p), nil
}
