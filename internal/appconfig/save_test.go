package appconfig

import (
	"os"
	"os/user"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/appconfig/confpath"
)

// 書き込み（Save）と既定パスの往復を検証する。
//
// 読み取り（config_test.go）と分けているのは、**見ているものが違う**ためである。
// あちらは YAML の解釈（未知のキー・範囲外の値・区切り）を見るのに対し、こちらは
// ファイルシステムに残るもの——パーミッション（security.md の表）・一時ファイルの
// 残骸・所有者——を見る。1 ファイル 300 行の上限（docs/ui/atomic-design.md）に
// 収める際の切れ目もここになる（Issue #110）。

// assertPerm は target のパーミッションを検証する。
func assertPerm(t *testing.T, target string, want os.FileMode) {
	t.Helper()
	fi, err := os.Stat(target)
	if err != nil {
		t.Fatalf("Stat() でエラー: %v", err)
	}
	if fi.Mode().Perm() != want {
		t.Errorf("%s のパーミッション = %o, want %o", target, fi.Mode().Perm(), want)
	}
}

// security.md の表どおりファイルは 600、ディレクトリは 700 になること。
//
// 既に緩い mode で存在するファイルは締め直す。os.WriteFile では perm が新規作成時に
// しか効かず、root が 644 で作ったものが直らないと、次に非 root で起動したときに
// 自分の設定を読み書きできない。
//
// 一方でディレクトリの mode を締め直すのは「対象ユーザーのホーム配下」に限る。
// --config で指された任意のディレクトリを root が 0700 にすると、本ツールへの sudo
// だけを許されたユーザーが任意のディレクトリを閉じられてしまう。ホーム配下の leaf を
// 締め直すことは confpath の TestMkdirOwnedFSTargets が担保する。
func TestSavePermissions(t *testing.T) {
	clearOwnerEnv(t)
	for _, tt := range []struct {
		name            string
		predir, prefile os.FileMode // 0 なら事前に作らない
		wantDir         os.FileMode // 0 なら confpath.DirMode
	}{
		{name: "多段の親ごと作る"},
		{name: "既存のディレクトリの mode はホーム外なので変えない", predir: 0o755, wantDir: 0o755},
		{name: "既存の緩いファイルを 600 に直す", predir: 0o700, prefile: 0o644},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "a", confpath.DirName)
			path := filepath.Join(dir, confpath.FileName)
			if tt.predir != 0 {
				if err := os.MkdirAll(dir, tt.predir); err != nil {
					t.Fatalf("準備に失敗: %v", err)
				}
			}
			if tt.prefile != 0 {
				if err := os.WriteFile(path, []byte("scan_depth: 1\n"), tt.prefile); err != nil {
					t.Fatalf("準備に失敗: %v", err)
				}
			}
			if err := Save(Default(), path); err != nil {
				t.Fatalf("Save() でエラー: %v", err)
			}
			assertPerm(t, path, confpath.FileMode)
			wantDir := tt.wantDir
			if wantDir == 0 {
				wantDir = confpath.DirMode
			}
			assertPerm(t, dir, wantDir)
		})
	}
}

// 全項目を指定した設定と、既定値だけの設定の両方が往復すること。
// Default() は ScanRoots / Labels が nil で、yaml.Marshal はこれを [] と書き出すため、
// 読み戻しで nil に戻る（normalize の len == 0 → nil）ことに依存している。
func TestSaveLoadRoundTrip(t *testing.T) {
	clearOwnerEnv(t)
	full := Config{
		ScanRoots:       []string{"/opt/runners", "/data/actions-runner"},
		ScanDepth:       4,
		RefreshInterval: 30,
		DiskThresholds:  DiskThresholds{Warn: 70, Critical: 85},
		AuditLog:        "/var/log/gsr-helper/audit.jsonl",
		Defaults: Defaults{
			NamePrefix:  "build01",
			InstallBase: "/srv/runners",
			Labels:      []string{"gpu", "cuda"},
			Ephemeral:   true,
		},
	}
	for _, want := range []Config{full, Default()} {
		path := filepath.Join(t.TempDir(), confpath.FileName)
		if err := Save(want, path); err != nil {
			t.Fatalf("Save() でエラー: %v", err)
		}
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile() でエラー: %v", err)
		}
		if !strings.HasPrefix(string(b), "#") {
			t.Errorf("先頭がヘッダコメントになっていない: %q", string(b[:min(len(b), 40)]))
		}
		got, err := Load(path)
		if err != nil {
			t.Fatalf("Load() でエラー: %v", err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("往復後 = %+v, want %+v", got, want)
		}
	}
}

// 検証に落ちた場合は本体も一時ファイルの残骸も残さないこと。
func TestSaveInvalidLeavesNoFile(t *testing.T) {
	clearOwnerEnv(t)
	dir := t.TempDir()
	cfg := Default()
	cfg.ScanDepth = 99

	if err := Save(cfg, filepath.Join(dir, confpath.FileName)); err == nil {
		t.Fatal("範囲外の scan_depth でエラーを返していない")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir() でエラー: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("ファイルが作られている: %v", entries)
	}
}

// path が空なら DefaultPath() を使うこと（Load / Save の既定経路）。
// configPath はホーム配下の絶対パスだけを尊重するため、t.TempDir() ではなく
// ホーム配下の一時ディレクトリを XDG_CONFIG_HOME に向ける。
func TestDefaultPathRoundTrip(t *testing.T) {
	t.Setenv(confpath.EnvSudoUser, "")
	self, err := user.Current()
	if err != nil {
		t.Fatalf("user.Current() でエラー: %v", err)
	}
	base, err := os.MkdirTemp(self.HomeDir, ".gsr-helper-test-")
	if err != nil {
		// 実行ユーザーのホームが書けない環境（ホーム未作成のサービスアカウント等）では
		// 既定の配置先そのものが使えないため、この経路は検証できない。
		t.Skipf("ホーム配下に一時ディレクトリを作れない: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(base) })
	t.Setenv(confpath.EnvXDGConfigHome, base)

	// 保存前は既定値が返る（ファイルが無くても Load は成功する）。
	if got, lerr := Load(""); lerr != nil || !reflect.DeepEqual(got, Default()) {
		t.Fatalf("保存前の Load() = %+v, %v, want %+v, nil", got, lerr, Default())
	}
	want := Default()
	want.ScanDepth = 5
	if serr := Save(want, ""); serr != nil {
		t.Fatalf("Save() でエラー: %v", serr)
	}
	got, err := Load("")
	if err != nil {
		t.Fatalf("Load() でエラー: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("往復後 = %+v, want %+v", got, want)
	}
}
