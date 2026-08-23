package tarball

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// zeroSum は「絶対に一致しない」チェックサム。取得そのものを見たい場面で使う。
const zeroSum = "0000000000000000000000000000000000000000000000000000000000000000"

// newBodyServer は body をそのまま返すサーバーを立てる。
//
// Content-Length を明示するのは、進捗の Total が総量不明（0）に落ちないようにするため。
func newBodyServer(t *testing.T, body []byte) *httptest.Server {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestFetchSavesAndVerifies(t *testing.T) {
	body := bytes.Repeat([]byte("runner"), 100)
	srv := newBodyServer(t, body)
	dir := filepath.Join(t.TempDir(), "downloads")

	// progress に nil を渡しても落ちないことを、正常系でそのまま確かめる。
	got, err := Fetch(t.Context(), srv.Client(), Info{
		URL:      srv.URL + "/actions-runner-linux-x64.tar.gz",
		Filename: "actions-runner-linux-x64.tar.gz",
		SHA256:   sha256Hex(body),
	}, dir, nil)
	if err != nil {
		t.Fatalf("Fetch がエラーを返した: %v", err)
	}

	want := filepath.Join(dir, "actions-runner-linux-x64.tar.gz")
	if got != want {
		t.Errorf("戻り値のパスが %q（期待 %q）", got, want)
	}
	if content := readFile(t, got); content != string(body) {
		t.Errorf("保存された中身が %d バイト（期待 %d バイト）", len(content), len(body))
	}
	if perm := mustPerm(t, got); perm != downloadFileMode {
		t.Errorf("保存されたファイルのパーミッションが %o（期待 %o）", perm, downloadFileMode)
	}

	// 大文字の期待値でも一致すること（比較は大文字小文字を区別しない）。
	if err := Verify(got, strings.ToUpper(sha256Hex(body))); err != nil {
		t.Errorf("Verify がエラーを返した: %v", err)
	}
}

// SHA-256 が合わない tarball は展開させてはならない（security.md）。
// 呼び出し側が誤って使えないよう、Fetch はファイルごと消す。
func TestFetchRemovesFileOnChecksumMismatch(t *testing.T) {
	body := []byte("改竄された tarball")
	srv := newBodyServer(t, body)
	dir := t.TempDir()

	got, err := Fetch(t.Context(), srv.Client(), Info{
		URL:      srv.URL + "/runner.tar.gz",
		Filename: "runner.tar.gz",
		SHA256:   zeroSum,
	}, dir, nil)
	if err == nil {
		t.Fatal("Fetch がチェックサム不一致を受け入れた")
	}
	if !errors.Is(err, ErrChecksumMismatch) {
		t.Fatalf("エラーが ErrChecksumMismatch ではない: %v", err)
	}
	if got != "" {
		t.Errorf("失敗時にパスが返っている: %q", got)
	}

	// 期待値と実際の値の両方を出す。手で突き合わせられないと原因が切り分けられない。
	msg := err.Error()
	for _, want := range []string{zeroSum, sha256Hex(body)} {
		if !strings.Contains(msg, want) {
			t.Errorf("エラー文言に %q が含まれない: %s", want, msg)
		}
	}

	if _, statErr := os.Lstat(filepath.Join(dir, "runner.tar.gz")); !errors.Is(statErr, os.ErrNotExist) {
		t.Errorf("検証に失敗したファイルが残っている: %v", statErr)
	}
}

func TestFetchRejectsNonOKStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)

	dir := t.TempDir()
	_, err := Fetch(t.Context(), srv.Client(), Info{
		URL:      srv.URL + "/runner.tar.gz",
		Filename: "runner.tar.gz",
		SHA256:   zeroSum,
	}, dir, nil)
	if !errors.Is(err, ErrUnexpectedStatus) {
		t.Fatalf("エラーが ErrUnexpectedStatus ではない: %v", err)
	}
	if !strings.Contains(err.Error(), "404") {
		t.Errorf("エラー文言にステータスが含まれない: %v", err)
	}
	if _, statErr := os.Lstat(filepath.Join(dir, "runner.tar.gz")); !errors.Is(statErr, os.ErrNotExist) {
		t.Errorf("失敗したのにファイルが残っている: %v", statErr)
	}
}

func TestFetchStopsOnContextCancel(t *testing.T) {
	// 終わらないレスポンス。受信の途中でキャンセルが効くことを見る。
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		chunk := bytes.Repeat([]byte("x"), 4096)
		for {
			select {
			case <-r.Context().Done():
				return
			default:
			}
			if _, err := w.Write(chunk); err != nil {
				return
			}
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
		}
	}))
	t.Cleanup(srv.Close)

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	dir := t.TempDir()
	var once sync.Once
	_, err := Fetch(ctx, srv.Client(), Info{
		URL:      srv.URL + "/runner.tar.gz",
		Filename: "runner.tar.gz",
		SHA256:   zeroSum,
	}, dir, func(_ Progress) {
		once.Do(cancel)
	})
	if err == nil {
		t.Fatal("キャンセルしたのに Fetch が成功した")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("エラーが context.Canceled を包んでいない: %v", err)
	}
	if _, statErr := os.Lstat(filepath.Join(dir, "runner.tar.gz")); !errors.Is(statErr, os.ErrNotExist) {
		t.Errorf("中断したのに書きかけのファイルが残っている: %v", statErr)
	}
}

func TestFetchReportsProgress(t *testing.T) {
	// io.Copy のバッファ（32KiB）を何度か跨ぐ大きさにして、複数回報告させる。
	body := bytes.Repeat([]byte("a"), 200*1024)
	srv := newBodyServer(t, body)

	var got []Progress
	path, err := Fetch(t.Context(), srv.Client(), Info{
		URL:      srv.URL + "/runner.tar.gz",
		Filename: "runner.tar.gz",
		SHA256:   sha256Hex(body),
	}, t.TempDir(), func(p Progress) {
		got = append(got, p)
	})
	if err != nil {
		t.Fatalf("Fetch がエラーを返した: %v", err)
	}
	if path == "" {
		t.Fatal("戻り値のパスが空")
	}

	if len(got) < 2 {
		t.Fatalf("進捗の報告が %d 回しかない（複数回に分かれるはず）", len(got))
	}
	var prev int64
	for i, p := range got {
		if p.Downloaded <= prev {
			t.Fatalf("%d 回目の Downloaded が %d で、前回の %d から増えていない", i, p.Downloaded, prev)
		}
		if p.Total != int64(len(body)) {
			t.Errorf("%d 回目の Total が %d（期待 %d）", i, p.Total, len(body))
		}
		prev = p.Downloaded
	}
	if last := got[len(got)-1].Downloaded; last != int64(len(body)) {
		t.Errorf("最後の Downloaded が %d（期待 %d）", last, len(body))
	}
}

func TestFetchRejectsBadInfo(t *testing.T) {
	body := []byte("runner")
	srv := newBodyServer(t, body)

	tests := []struct {
		name string
		in   Info
		// wantErr は errors.Is で突き合わせる番兵。
		wantErr error
	}{
		{
			name:    "URL が空",
			in:      Info{URL: "", Filename: "runner.tar.gz", SHA256: zeroSum},
			wantErr: ErrNoURL,
		},
		{
			name:    "チェックサムが空",
			in:      Info{URL: srv.URL, Filename: "runner.tar.gz", SHA256: ""},
			wantErr: ErrNoChecksum,
		},
		{
			name:    "ファイル名が区切りを含む",
			in:      Info{URL: srv.URL, Filename: "../evil.tar.gz", SHA256: zeroSum},
			wantErr: ErrUnsafePath,
		},
		{
			name:    "ファイル名が親ディレクトリ",
			in:      Info{URL: srv.URL, Filename: "..", SHA256: zeroSum},
			wantErr: ErrUnsafePath,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()

			_, err := Fetch(t.Context(), srv.Client(), tt.in, dir, nil)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("エラーが %v ではない: %v", tt.wantErr, err)
			}

			entries, readErr := os.ReadDir(dir)
			if readErr != nil {
				t.Fatalf("%s の読み取りに失敗した: %v", dir, readErr)
			}
			if len(entries) != 0 {
				t.Errorf("保存先に %d 件書き出されている", len(entries))
			}
		})
	}
}

// Filename が空なら URL の末尾要素を使う。gh の Download は常に埋めてくるが、
// 呼び出し側が詰め替えを忘れても保存先が壊れないようにしておく。
func TestFetchFallsBackToURLBasename(t *testing.T) {
	body := []byte("runner")
	srv := newBodyServer(t, body)
	dir := t.TempDir()

	got, err := Fetch(t.Context(), srv.Client(), Info{
		URL:      srv.URL + "/download/actions-runner.tar.gz",
		Filename: "",
		SHA256:   sha256Hex(body),
	}, dir, nil)
	if err != nil {
		t.Fatalf("Fetch がエラーを返した: %v", err)
	}
	if want := filepath.Join(dir, "actions-runner.tar.gz"); got != want {
		t.Errorf("保存先が %q（期待 %q）", got, want)
	}
}

// c が nil でも既定のクライアントで取得できること。
func TestFetchUsesDefaultClient(t *testing.T) {
	body := []byte("runner")
	srv := newBodyServer(t, body)

	got, err := Fetch(t.Context(), nil, Info{
		URL:      srv.URL + "/runner.tar.gz",
		Filename: "runner.tar.gz",
		SHA256:   sha256Hex(body),
	}, t.TempDir(), nil)
	if err != nil {
		t.Fatalf("Fetch がエラーを返した: %v", err)
	}
	if readFile(t, got) != string(body) {
		t.Error("既定のクライアントで取得した中身が一致しない")
	}
}
