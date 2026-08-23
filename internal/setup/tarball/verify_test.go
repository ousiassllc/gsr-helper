package tarball

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVerify(t *testing.T) {
	body := []byte("actions-runner tarball の中身")
	dir := t.TempDir()
	path := filepath.Join(dir, "runner.tar.gz")
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatalf("%s の作成に失敗した: %v", path, err)
	}
	missing := filepath.Join(dir, "no-such.tar.gz")

	tests := []struct {
		name string
		path string
		want string
		// wantErr は errors.Is で突き合わせる番兵。nil なら成功を期待する。
		wantErr error
	}{
		{
			name:    "一致する",
			path:    path,
			want:    sha256Hex(body),
			wantErr: nil,
		},
		{
			name:    "大文字の期待値でも一致する",
			path:    path,
			want:    strings.ToUpper(sha256Hex(body)),
			wantErr: nil,
		},
		{
			name:    "前後の空白は無視する",
			path:    path,
			want:    "  " + sha256Hex(body) + "\n",
			wantErr: nil,
		},
		{
			name:    "一致しない",
			path:    path,
			want:    zeroSum,
			wantErr: ErrChecksumMismatch,
		},
		{
			name:    "期待値が空",
			path:    path,
			want:    "",
			wantErr: ErrNoChecksum,
		},
		{
			name:    "ファイルが無い",
			path:    missing,
			want:    sha256Hex(body),
			wantErr: os.ErrNotExist,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Verify(tt.path, tt.want)
			if tt.wantErr == nil {
				if err != nil {
					t.Fatalf("Verify がエラーを返した: %v", err)
				}
				return
			}
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("エラーが %v ではない: %v", tt.wantErr, err)
			}
		})
	}
}

// 不一致の文言は、期待値と実際の値の両方を出す。
func TestVerifyMismatchMessage(t *testing.T) {
	body := []byte("runner")
	path := filepath.Join(t.TempDir(), "runner.tar.gz")
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatalf("%s の作成に失敗した: %v", path, err)
	}

	err := Verify(path, zeroSum)
	if err == nil {
		t.Fatal("Verify が不一致を受け入れた")
	}
	for _, want := range []string{path, zeroSum, sha256Hex(body)} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("エラー文言に %q が含まれない: %s", want, err)
		}
	}
}
