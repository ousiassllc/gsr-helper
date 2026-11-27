package runner

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// blockLimit は「ブロックしないこと」を表明する上限。正常なら一瞬で返る処理が
// 上限内に返らなかったことだけを失敗とし、固定時間の待ちはしない。
const blockLimit = 5 * time.Second

// hostile は runner 実行ユーザーが仕込みうる細工。
var hostile = []struct {
	name    string
	make    func(path string) error
	wantErr error
}{
	{"FIFO", func(p string) error { return syscall.Mkfifo(p, 0o600) }, errNotRegular},
	{"上限超", func(p string) error {
		return os.WriteFile(p, []byte(strings.Repeat("a", maxSmallFileBytes+1)), 0o600)
	}, errTooLarge},
	{"/dev/zero へのリンク", func(p string) error { return os.Symlink("/dev/zero", p) }, syscall.ELOOP},
	{"通常ファイルへのリンク", func(p string) error {
		if err := os.WriteFile(p+".real", []byte("2.311.0\n"), 0o600); err != nil {
			return err
		}
		return os.Symlink(p+".real", p)
	}, syscall.ELOOP},
	{"ディレクトリ", func(p string) error { return os.Mkdir(p, 0o700) }, errNotRegular},
}

// TestDiscoveryReadsSurviveHostileFiles は、.runner / bin/runnerversion / .service が
// 細工されていてもブロックもメモリ確保もせず、縮退（空文字・エラー）で返ることを見る。
func TestDiscoveryReadsSurviveHostileFiles(t *testing.T) {
	for _, tt := range hostile {
		t.Run(tt.name, func(t *testing.T) {
			dir := mkDir(t, filepath.Join(t.TempDir(), "r"), map[string]string{"bin/.keep": ""})
			for _, name := range []string{".runner", ".service", "bin/runnerversion"} {
				if err := tt.make(filepath.Join(dir, name)); err != nil {
					t.Fatal(err)
				}
			}
			done := make(chan error, 1)
			go func() {
				if got := readUnitName(dir) + readVersion(dir); got != "" {
					t.Errorf("readUnitName+readVersion = %q, want 空文字", got)
				}
				_, err := LoadConfig(dir)
				done <- err
			}()
			select {
			case err := <-done:
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("LoadConfig err = %v, want %v", err, tt.wantErr)
				}
			case <-time.After(blockLimit):
				t.Fatalf("%v 以内に返りませんでした（FIFO の open か無限読み取り）", blockLimit)
			}
		})
	}
}

func TestReadSmallFileLimitBoundary(t *testing.T) {
	path := filepath.Join(t.TempDir(), "f")
	if err := os.WriteFile(path, []byte(strings.Repeat("a", maxSmallFileBytes)), 0o600); err != nil {
		t.Fatal(err)
	}
	if b, err := readSmallFile(path); err != nil || len(b) != maxSmallFileBytes {
		t.Errorf("上限ちょうど: len = %d, err = %v, want 読めること", len(b), err)
	}
}
