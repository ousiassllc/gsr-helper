package appconfig

import (
	"os"
	"os/user"
	"path/filepath"
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/appconfig/confpath"
)

// Exists は初回起動の判定に使うため、「無い」と「確認できない」を区別する。
// Load はどちらも既定値に丸めてしまうので、その差はここでしか検証できない。
func TestExistsWithExplicitPath(t *testing.T) {
	clearOwnerEnv(t)

	dir := t.TempDir()
	path := filepath.Join(dir, confpath.FileName)

	got, err := Exists(path)
	if err != nil {
		t.Fatalf("作成前の Exists() でエラー: %v", err)
	}
	if got {
		t.Errorf("作成前の Exists() = true, want false")
	}

	if werr := os.WriteFile(path, []byte("scan_depth: 2\n"), confpath.FileMode); werr != nil {
		t.Fatalf("設定ファイルの作成に失敗: %v", werr)
	}

	got, err = Exists(path)
	if err != nil {
		t.Fatalf("作成後の Exists() でエラー: %v", err)
	}
	if !got {
		t.Errorf("作成後の Exists() = false, want true")
	}
}

// 権限で確認できない場合を「無い」に丸めると、ウィザードが毎回立ち上がったうえで
// 書き込みも失敗し続ける。エラーとして返すことを検証する。
func TestExistsReportsStatFailure(t *testing.T) {
	clearOwnerEnv(t)

	if os.Geteuid() == 0 {
		t.Skip("root は権限の制限を受けないため、この経路は検証できない")
	}

	dir := filepath.Join(t.TempDir(), "locked")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatalf("ディレクトリの作成に失敗: %v", err)
	}
	if err := os.Chmod(dir, 0o000); err != nil {
		t.Fatalf("パーミッションの変更に失敗: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	got, err := Exists(filepath.Join(dir, confpath.FileName))
	if err == nil {
		t.Fatalf("Exists() = %v, nil, want エラー", got)
	}
	if got {
		t.Errorf("Exists() = true, want false")
	}
}

// path が空なら DefaultPath() を使うこと。配置先の決定が呼び出し側へ漏れていない
// ことの確認であり、os.Stat(DefaultPath()) を各所に書かせないための API である。
func TestExistsUsesDefaultPath(t *testing.T) {
	t.Setenv(confpath.EnvSudoUser, "")

	self, err := user.Current()
	if err != nil {
		t.Fatalf("user.Current() でエラー: %v", err)
	}
	base, err := os.MkdirTemp(self.HomeDir, ".gsr-helper-test-")
	if err != nil {
		t.Skipf("ホーム配下に一時ディレクトリを作れない: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(base) })
	t.Setenv(confpath.EnvXDGConfigHome, base)

	got, err := Exists("")
	if err != nil {
		t.Fatalf("保存前の Exists() でエラー: %v", err)
	}
	if got {
		t.Errorf("保存前の Exists() = true, want false")
	}

	if serr := Save(Default(), ""); serr != nil {
		t.Fatalf("Save() でエラー: %v", serr)
	}

	got, err = Exists("")
	if err != nil {
		t.Fatalf("保存後の Exists() でエラー: %v", err)
	}
	if !got {
		t.Errorf("保存後の Exists() = false, want true")
	}
}
