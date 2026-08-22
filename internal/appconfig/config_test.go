package appconfig

import (
	"bytes"
	"os"
	"os/user"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ousiassllc/gsr-helper/internal/appconfig/confpath"
)

// clearOwnerEnv は所有者解決が実行環境の SUDO_USER に左右されないようにする。
func clearOwnerEnv(t *testing.T) {
	t.Helper()
	t.Setenv(confpath.EnvSudoUser, "")
	t.Setenv(confpath.EnvXDGConfigHome, "")
}

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

// ファイルが無い場合と、空・コメントのみの場合は既定値になること。どちらも初回
// 起動で普通に起こる状態であり、異常として扱うと起動できなくなる。
func TestLoadReturnsDefault(t *testing.T) {
	for _, name := range []string{"nonexistent.yaml", "empty.yaml", "comment-only.yaml"} {
		t.Run(name, func(t *testing.T) {
			got, err := Load(filepath.Join("testdata", name))
			if err != nil {
				t.Fatalf("Load() でエラー: %v", err)
			}
			if !reflect.DeepEqual(got, Default()) {
				t.Errorf("Load() = %+v, want %+v", got, Default())
			}
		})
	}
}

func TestLoadFull(t *testing.T) {
	got, err := Load(filepath.Join("testdata", "full.yaml"))
	if err != nil {
		t.Fatalf("Load() でエラー: %v", err)
	}
	want := Config{
		ScanRoots:       []string{"/opt/runners", "/data/actions-runner"},
		ScanDepth:       2,
		RefreshInterval: 3,
		DiskThresholds:  DiskThresholds{Warn: 80, Critical: 90},
		AuditLog:        "/var/log/gsr-helper/audit.jsonl",
		Defaults: Defaults{
			NamePrefix:  "",
			InstallBase: "/opt/runners",
			Labels:      []string{"self-hosted-extra"},
			Ephemeral:   false,
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Load() = %+v, want %+v", got, want)
	}
}

// 部分指定では書いた項目だけが変わり、残りは既定値になること。
func TestLoadPartialKeepsDefaults(t *testing.T) {
	got, err := Load(filepath.Join("testdata", "partial.yaml"))
	if err != nil {
		t.Fatalf("Load() でエラー: %v", err)
	}
	want := Default()
	want.RefreshInterval = 10
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Load() = %+v, want %+v", got, want)
	}
}

// 未知のキーは入れ子でも弾き、2 つ目のドキュメントや範囲外の値も黙って通さないこと。
func TestLoadErrors(t *testing.T) {
	for _, name := range []string{
		"invalid.yaml", "unknown-key.yaml", "unknown-nested-key.yaml",
		"type-mismatch.yaml", "multi-doc.yaml", "out-of-range.yaml",
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join("testdata", name)
			_, err := Load(path)
			if err == nil {
				t.Fatalf("Load(%s) がエラーを返していない", path)
			}
			if !strings.Contains(err.Error(), path) {
				t.Errorf("エラー文に path が含まれていない: %v", err)
			}
		})
	}
}

// 手編集で最も出やすい打ち間違いのエラー文は日本語で、Go の内部型名を出さないこと。
func TestLoadUnknownKeyMessage(t *testing.T) {
	_, err := Load(filepath.Join("testdata", "unknown-key.yaml"))
	if err == nil {
		t.Fatal("未知のキーでエラーを返していない")
	}
	if !strings.Contains(err.Error(), "不明なキーです: scan_dept（2 行目）") {
		t.Errorf("未知のキーの説明が日本語になっていない: %v", err)
	}
	if strings.Contains(err.Error(), "appconfig.Config") {
		t.Errorf("内部型名が露出している: %v", err)
	}
}

// --config /dev/zero のような指定でメモリを食い潰さないこと。
func TestLoadRejectsHugeFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), confpath.FileName)
	if err := os.WriteFile(path, bytes.Repeat([]byte("#"), maxConfigSize+1), confpath.FileMode); err != nil {
		t.Fatalf("準備に失敗: %v", err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("上限を超えるファイルでエラーを返していない")
	}
}

func TestRefreshDuration(t *testing.T) {
	if got := (Config{RefreshInterval: 3}).RefreshDuration(); got != 3*time.Second {
		t.Errorf("RefreshDuration() = %v, want %v", got, 3*time.Second)
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

// path が空なら DefaultPath() を使うこと（Load / Save / Exists の既定経路）。
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

	if got, eerr := Exists(""); got || eerr != nil {
		t.Fatalf("Exists() = %v, %v, want false, nil", got, eerr)
	}
	want := Default()
	want.ScanDepth = 5
	if serr := Save(want, ""); serr != nil {
		t.Fatalf("Save() でエラー: %v", serr)
	}
	if got, eerr := Exists(""); !got || eerr != nil {
		t.Errorf("Exists() = %v, %v, want true, nil", got, eerr)
	}
	got, err := Load("")
	if err != nil {
		t.Fatalf("Load() でエラー: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("往復後 = %+v, want %+v", got, want)
	}
}
