// Package runner はホスト上の self-hosted runner を探索し、設定・プロセス・systemd ユニットの状態を突き合わせる。
package runner

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Config は runner ディレクトリ直下の .runner ファイルの内容。
type Config struct {
	AgentID       int64  `json:"agentId"`
	AgentName     string `json:"agentName"`
	PoolID        int64  `json:"poolId"`
	PoolName      string `json:"poolName"`
	ServerURL     string `json:"serverUrl"`
	GitHubURL     string `json:"gitHubUrl"`
	WorkFolder    string `json:"workFolder"`
	Ephemeral     bool   `json:"ephemeral"`
	DisableUpdate bool   `json:"disableUpdate"`
}

// IsRunnerDir は dir が runner のルートディレクトリかを判定する。
func IsRunnerDir(dir string) bool {
	fi, err := os.Stat(filepath.Join(dir, ".runner"))
	return err == nil && fi.Mode().IsRegular()
}

// parseConfig は .runner の中身をパースする。
//
// .runner は runner 本体 (.NET) が書き出すため UTF-8 BOM が付くことがある。
func parseConfig(b []byte) (Config, error) {
	var c Config
	b = bytes.TrimPrefix(b, []byte("\xef\xbb\xbf"))
	if err := json.Unmarshal(b, &c); err != nil {
		return Config{}, fmt.Errorf(".runner の JSON 解析に失敗しました: %w", err)
	}
	return c, nil
}

// LoadConfig は <dir>/.runner を読み込む。
// dir は Discover が解決済みの runner ディレクトリであること（呼び出し側の事前条件）。
//
// エラーは読み込み失敗・解析失敗のいずれも `<dir>: <原因>` の形にする。前置するのは
// `.runner` のパスではなくディレクトリで、Discover のスコープ判定失敗と同じ形に
// そろえてある（docs/architecture/data-model.md の「Result の警告」表）。
// runner を識別する単位はディレクトリであり、固定名の `.runner` を足しても
// どの runner かの手がかりは増えない。
func LoadConfig(dir string) (Config, error) {
	path := filepath.Join(dir, ".runner")
	//nolint:gosec // dir は Discover が解決した runner ディレクトリという事前条件（直上の doc コメント）に依存。path は dir + 固定名 ".runner"。
	b, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("%s: .runner の読み込みに失敗しました: %w", dir, err)
	}
	cfg, err := parseConfig(b)
	if err != nil {
		return Config{}, fmt.Errorf("%s: %w", dir, err)
	}
	return cfg, nil
}

// readVersion は <dir>/bin/runnerversion を読む。無ければ空文字を返す。
func readVersion(dir string) string {
	//nolint:gosec // 探索済み runner ディレクトリ + 固定パス "bin/runnerversion"。パスに外部入力は入らない。
	b, err := os.ReadFile(filepath.Join(dir, "bin", "runnerversion"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// readUnitName は <dir>/.service を読む。svc.sh install が systemd ユニット名を
// ここに書き出すため、ディレクトリとユニットを確実に紐付けられる。
// サービス化されていなければ空文字を返す。
func readUnitName(dir string) string {
	//nolint:gosec // 探索済み runner ディレクトリ + 固定名 ".service"。パスに外部入力は入らない。
	b, err := os.ReadFile(filepath.Join(dir, ".service"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// resolveWorkDir は workFolder を絶対パスに正規化する。
// workFolder は既定で "_work" のような runner ディレクトリからの相対パス。
func resolveWorkDir(dir, workFolder string) string {
	if workFolder == "" {
		workFolder = "_work"
	}
	if filepath.IsAbs(workFolder) {
		return filepath.Clean(workFolder)
	}
	return filepath.Join(dir, workFolder)
}
