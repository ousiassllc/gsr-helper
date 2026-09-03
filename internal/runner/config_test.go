package runner

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestParseConfig(t *testing.T) {
	tests := []struct {
		name, file string
		want       Config
		wantErr    bool
	}{
		{name: "正常", file: "normal.json", want: Config{
			AgentID: 42, AgentName: "host-1", PoolID: 7, PoolName: "Default",
			ServerURL:  "https://pipelines.actions.githubusercontent.com/AbCdEf",
			GitHubURL:  "https://github.com/myorg/myrepo",
			WorkFolder: "_work",
		}},
		{name: "UTF-8 BOM 付き", file: "bom.json", want: Config{
			AgentID: 1, AgentName: "bom-runner",
			GitHubURL: "https://github.com/orgs/myorg", WorkFolder: "_work",
		}},
		{name: "キー欠落はゼロ値でエラーにしない", file: "missing_keys.json", want: Config{AgentName: "only-name"}},
		{name: "null は全てゼロ値", file: "null.json"},
		{name: "絶対パスの workFolder", file: "abs_workfolder.json", want: Config{AgentName: "abs-runner", WorkFolder: "/data/work/"}},
		{name: "ephemeral / disableUpdate", file: "flags.json", want: Config{AgentName: "eph", Ephemeral: true, DisableUpdate: true}},
		{name: "不正 JSON", file: "invalid_json.json", wantErr: true},
		{name: "空ファイル", file: "empty.json", wantErr: true},
		{name: "配列", file: "array.json", wantErr: true},
	}
	for _, tt := range tests {
		got, err := parseConfig(fixture(t, "dotrunner/"+tt.file))
		if (err != nil) != tt.wantErr {
			t.Errorf("%s: err = %v, wantErr = %v", tt.name, err, tt.wantErr)
		} else if got != tt.want {
			t.Errorf("%s: got %+v, want %+v", tt.name, got, tt.want)
		}
	}
}

func TestResolveWorkDir(t *testing.T) {
	tests := []struct{ name, workFolder, want string }{
		{"相対パス", "work", "/opt/r/work"},
		{"空なら既定の _work", "", "/opt/r/_work"},
		{"絶対パス", "/data/work/", "/data/work"},
		{".. を含む相対パス", "../shared/_work", "/opt/shared/_work"},
	}
	for _, tt := range tests {
		if got := resolveWorkDir("/opt/r", tt.workFolder); got != tt.want {
			t.Errorf("%s: got %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestIsRunnerDir(t *testing.T) {
	base := t.TempDir()
	// .runner がディレクトリの場合は runner ディレクトリではない。
	asDir := mkDir(t, filepath.Join(base, "asdir", ".runner"), nil)

	tests := []struct {
		name, dir string
		want      bool
	}{
		{".runner あり", mkRunner(t, filepath.Join(base, "with")), true},
		{".runner 無し", mkDir(t, filepath.Join(base, "without"), nil), false},
		{".runner がディレクトリ", filepath.Dir(asDir), false},
		{"存在しないパス", filepath.Join(base, "nope"), false},
	}
	for _, tt := range tests {
		if got := IsRunnerDir(tt.dir); got != tt.want {
			t.Errorf("%s: got %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestReadVersionAndUnitName(t *testing.T) {
	dir := mkDir(t, filepath.Join(t.TempDir(), "r"), map[string]string{
		"bin/runnerversion": "2.311.0\n",
		".service":          " actions.runner.myorg.host-1.service \n",
	})
	empty := mkDir(t, filepath.Join(t.TempDir(), "e"), nil)

	if got := readVersion(dir); got != "2.311.0" {
		t.Errorf("readVersion = %q", got)
	}
	if got := readUnitName(dir); got != "actions.runner.myorg.host-1.service" {
		t.Errorf("readUnitName = %q", got)
	}
	// ファイルが無い場合は空文字（エラーにしない）。
	if got, got2 := readVersion(empty), readUnitName(empty); got != "" || got2 != "" {
		t.Errorf("ファイル不在で (%q, %q), want 空文字", got, got2)
	}
}

// LoadConfig のエラーは読み込み失敗・解析失敗のいずれも `<dir>: <原因>` の形にする
// （[データモデル]の「Result の警告」表）。`<dir>/.runner` を前置すると、同じ行が
// 定めるスコープ判定失敗（Discover 側でディレクトリを前置する）と形が食い違う。
//
// [データモデル]: ../../docs/architecture/data-model.md
func TestLoadConfigErrorPrefix(t *testing.T) {
	base := t.TempDir()
	missing := mkDir(t, filepath.Join(base, "missing"), nil)
	broken := mkDir(t, filepath.Join(base, "broken"), map[string]string{".runner": "{"})

	tests := []struct{ name, dir, wantPrefix string }{
		{".runner が無い", missing, missing + ": .runner の読み込みに失敗しました: "},
		{".runner が壊れている", broken, broken + ": .runner の JSON 解析に失敗しました: "},
	}
	for _, tt := range tests {
		_, err := LoadConfig(tt.dir)
		if err == nil {
			t.Errorf("%s: err = nil, want エラー", tt.name)
			continue
		}
		if !strings.HasPrefix(err.Error(), tt.wantPrefix) {
			t.Errorf("%s: err = %q, want prefix %q", tt.name, err, tt.wantPrefix)
		}
	}
}
