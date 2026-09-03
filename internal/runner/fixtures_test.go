package runner

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// 複数のテストファイルから使う共有フィクスチャをここに集約する。
// attach などの純粋関数は正規化済みの入力を受け取るので、実在しないパスを渡してよい。
const (
	d1 = "/opt/actions-runner-1"
	d2 = "/opt/actions-runner-2"
	u1 = "actions.runner.myorg.host-1.service"
	u2 = "actions.runner.myorg.host-2.service"
)

var zero time.Time // 起動時刻を問わないケース用

// 以下のヘルパでテーブルを短く書く。
func rn(dir, unit string) Runner { return Runner{Dir: dir, UnitName: unit} }

func pr(pid int, kind ProcKind, dir string, started time.Time) Process {
	return Process{PID: pid, Kind: kind, Dir: dir, Started: started}
}

func sv(unit, workDir string) SvcState {
	return SvcState{Unit: unit, WorkingDir: workDir, Load: "loaded", Active: "active"}
}

func unitNames(units []SvcState) []string {
	out := make([]string, 0, len(units))
	for _, u := range units {
		out = append(out, u.Unit)
	}
	return out
}

// fixture は testdata 配下のフィクスチャを読む。
func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("フィクスチャ %s の読み込みに失敗しました: %v", name, err)
	}
	return b
}

// mkDir は dir を作り、files の相対パス（"bin/runnerversion" のような
// サブディレクトリを含んでよい）にその内容を書く。
func mkDir(t *testing.T, dir string, files map[string]string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("ディレクトリの作成に失敗しました: %v", err)
	}
	for name, body := range files {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatalf("ディレクトリの作成に失敗しました: %v", err)
		}
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatalf("ファイルの書き込みに失敗しました: %v", err)
		}
	}
	return dir
}

// mkRunner は .runner を持つディレクトリを作る。
func mkRunner(t *testing.T, dir string) string {
	t.Helper()
	return mkDir(t, dir, map[string]string{".runner": `{"agentName":"x"}`})
}
