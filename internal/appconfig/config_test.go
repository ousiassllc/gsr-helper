package appconfig

import (
	"bytes"
	"os"
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

// 末尾に --- だけが残っているファイルは 1 ドキュメントとして受け付けること。
//
// 手編集を前提にした形式（non-functional.md）なので、区切りだけが残る状態は
// 普通に起こる。区切りの後に中身が無いなら「2 つ目のドキュメントを黙って
// 捨てる」ことにはならないので弾く理由がない。
func TestLoadAllowsTrailingDocumentSeparator(t *testing.T) {
	for name, body := range map[string]string{
		"末尾に区切りだけ":    "refresh_interval: 5\n---\n",
		"末尾に区切りとコメント": "refresh_interval: 5\n---\n# あとで書く\n",
		"末尾に区切りが 2 つ": "refresh_interval: 5\n---\n---\n",
		"先頭と末尾に区切り":   "---\nrefresh_interval: 5\n---\n",
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), confpath.FileName)
			if err := os.WriteFile(path, []byte(body), confpath.FileMode); err != nil {
				t.Fatalf("準備に失敗: %v", err)
			}
			got, err := Load(path)
			if err != nil {
				t.Fatalf("Load() でエラー: %v", err)
			}
			want := Default()
			want.RefreshInterval = 5
			if !reflect.DeepEqual(got, want) {
				t.Errorf("Load() = %+v, want %+v", got, want)
			}
		})
	}
}
