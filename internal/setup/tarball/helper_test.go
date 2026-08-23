package tarball

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

// 実物の tarball はコミットしない。異常系（".." を含むエントリ、外を指すリンク）は
// そもそも普通のツールでは作れないため、fixture は毎回コードから組み立てる。

// tarEntry はテスト用の tar.gz に入れるエントリ 1 件。
type tarEntry struct {
	// name はアーカイブ内の名前。
	name string
	// typeflag は tar のエントリ種別。
	typeflag byte
	// mode はパーミッション。
	mode int64
	// body は通常ファイルの中身。
	body string
	// linkname はリンクの向き先。
	linkname string
}

// regEntry は通常ファイルのエントリを作る。
func regEntry(name string, mode int64, body string) tarEntry {
	return tarEntry{name: name, typeflag: tar.TypeReg, mode: mode, body: body, linkname: ""}
}

// dirEntry はディレクトリのエントリを作る。
func dirEntry(name string, mode int64) tarEntry {
	return tarEntry{name: name, typeflag: tar.TypeDir, mode: mode, body: "", linkname: ""}
}

// symEntry はシンボリックリンクのエントリを作る。
func symEntry(name, linkname string) tarEntry {
	return tarEntry{name: name, typeflag: tar.TypeSymlink, mode: 0o777, body: "", linkname: linkname}
}

// linkEntry はハードリンクのエントリを作る。
func linkEntry(name, linkname string) tarEntry {
	return tarEntry{name: name, typeflag: tar.TypeLink, mode: 0o644, body: "", linkname: linkname}
}

// fifoEntry は runner の tarball には現れない種別のエントリを作る。
func fifoEntry(name string) tarEntry {
	return tarEntry{name: name, typeflag: tar.TypeFifo, mode: 0o644, body: "", linkname: ""}
}

// tarGzBytes は entries を tar.gz にして返す。
func tarGzBytes(t *testing.T, entries []tarEntry) []byte {
	t.Helper()

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)

	for _, e := range entries {
		hdr := &tar.Header{
			Name:     e.name,
			Typeflag: e.typeflag,
			Mode:     e.mode,
			Size:     int64(len(e.body)),
			Linkname: e.linkname,
			Format:   tar.FormatPAX,
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("tar ヘッダ %q の書き出しに失敗した: %v", e.name, err)
		}
		if e.body != "" {
			if _, err := tw.Write([]byte(e.body)); err != nil {
				t.Fatalf("tar 本文 %q の書き出しに失敗した: %v", e.name, err)
			}
		}
	}

	if err := tw.Close(); err != nil {
		t.Fatalf("tar のクローズに失敗した: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("gzip のクローズに失敗した: %v", err)
	}
	return buf.Bytes()
}

// writeTarGz は entries から tar.gz を作り、そのパスを返す。
func writeTarGz(t *testing.T, entries []tarEntry) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "actions-runner.tar.gz")
	if err := os.WriteFile(path, tarGzBytes(t, entries), 0o600); err != nil {
		t.Fatalf("%s の書き出しに失敗した: %v", path, err)
	}
	return path
}

// sha256Hex は b の SHA-256 を 16 進小文字で返す。
func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// readFile はテスト対象が書き出したファイルの中身を返す。
func readFile(t *testing.T, path string) string {
	t.Helper()

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%s の読み取りに失敗した: %v", path, err)
	}
	return string(b)
}

// mustPerm はファイルのパーミッションを返す。
func mustPerm(t *testing.T, path string) os.FileMode {
	t.Helper()

	fi, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("%s の Lstat に失敗した: %v", path, err)
	}
	return fi.Mode().Perm()
}
