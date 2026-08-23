package envfile_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/config/envfile"
)

// 読み書きを往復してもコメントと空行が保たれること。ディスクを経由しても
// Parse / Bytes と同じ結果になることを見る。
func TestLoadSaveRoundTrip(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte(sample), 0o600); err != nil {
		t.Fatalf(".env の作成に失敗: %v", err)
	}

	f, err := envfile.Load(path)
	if err != nil {
		t.Fatalf("Load() でエラー: %v", err)
	}
	f.Set("https_proxy", "http://new-proxy:3128")

	if err := envfile.Save(f, path); err != nil {
		t.Fatalf("Save() でエラー: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("読み込みに失敗: %v", err)
	}
	if string(got) != f.String() {
		t.Errorf("書き出した内容 = %q, want %q", got, f.String())
	}

	again, err := envfile.Load(path)
	if err != nil {
		t.Fatalf("再 Load() でエラー: %v", err)
	}
	if v, _ := again.Get("https_proxy"); v != "http://new-proxy:3128" {
		t.Errorf("再読み込みの https_proxy = %q", v)
	}
	if v, _ := again.Get("PATH"); v != "/usr/local/bin:/usr/bin:/bin" {
		t.Errorf("変更していない PATH が変わっている: %q", v)
	}
}

// .env が無いのは runner の普通の状態であり、空として扱うこと。
// 異常にすると編集を始められない。
func TestLoadMissingFileIsEmpty(t *testing.T) {
	t.Parallel()

	f, err := envfile.Load(filepath.Join(t.TempDir(), ".env"))
	if err != nil {
		t.Fatalf("Load() でエラー: %v", err)
	}
	if got := f.String(); got != "" {
		t.Errorf("Load() = %q, want 空", got)
	}
}

// 上限を超える .env は読まないこと。
func TestLoadRejectsTooLarge(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), ".env")
	big := make([]byte, envfile.MaxSize+1)
	for i := range big {
		big[i] = 'x'
	}
	if err := os.WriteFile(path, big, 0o600); err != nil {
		t.Fatalf(".env の作成に失敗: %v", err)
	}

	if _, err := envfile.Load(path); err == nil {
		t.Fatal("Load() = nil, want エラー")
	}
}

// .path の往復。末尾の改行はちょうど 1 つになること。
func TestPathFileRoundTrip(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), ".path")

	// 無ければ空。
	p, err := envfile.LoadPath(path)
	if err != nil || p.Value != "" {
		t.Fatalf("不在の LoadPath() = %+v, %v, want 空, nil", p, err)
	}

	p.Value = "/usr/local/bin:/usr/bin:/bin"
	if err := envfile.SavePath(p, path); err != nil {
		t.Fatalf("SavePath() でエラー: %v", err)
	}

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("読み込みに失敗: %v", err)
	}
	if want := "/usr/local/bin:/usr/bin:/bin\n"; string(b) != want {
		t.Errorf("書き出した内容 = %q, want %q", b, want)
	}

	got, err := envfile.LoadPath(path)
	if err != nil || got.Value != p.Value {
		t.Errorf("LoadPath() = %+v, %v, want %+v, nil", got, err, p)
	}
}

// 改行を含む値は書き込まないこと。.path は 1 行でなければならない。
func TestSavePathRejectsMultiLine(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, ".path")

	for name, v := range map[string]string{"LF": "/a\n/b", "CR": "/a\r/b"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			err := envfile.SavePath(envfile.PathFile{Value: v}, path)
			if !errors.Is(err, envfile.ErrMultiLine) {
				t.Fatalf("SavePath() のエラー = %v, want ErrMultiLine", err)
			}
		})
	}

	if _, err := os.Lstat(path); err == nil {
		t.Error("拒んだのにファイルが作られている")
	}
}
