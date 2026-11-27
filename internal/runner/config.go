// Package runner はホスト上の self-hosted runner を探索し、設定・プロセス・systemd ユニットの状態を突き合わせる。
package runner

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// maxSmallFileBytes は runner ディレクトリ直下で読む小さなファイル（.runner /
// bin/runnerversion / .service）の上限。いずれも実際は数百バイト〜数 KB である。
const maxSmallFileBytes = 16 << 10

var (
	errNotRegular = errors.New("通常ファイルではありません")
	errTooLarge   = errors.New("ファイルが大きすぎます")
)

// readSmallFile は path を最大 maxSmallFileBytes まで読む。
//
// runner ディレクトリは runner 実行ユーザーが書き換えられるため、os.ReadFile では
// /dev/zero へのリンク（EOF が来ずメモリを食い続ける）や FIFO（open(2) が書き手を
// 待って戻らない）が成立する。O_NOFOLLOW でリンクを辿らず、O_NONBLOCK で FIFO の
// open を即座に返させ、開いた fd 上で通常ファイルかを確かめる（確認と open を別操作に
// すると TOCTOU になる）。internal/config/fileio に同じ守りがあるが、runner は他の
// ドメインパッケージに依存しないため再利用せず最小限をここに置く（docs/components/overview.md）。
func readSmallFile(path string) ([]byte, error) {
	//nolint:gosec // 呼び出し元は探索済み runner ディレクトリ + 固定名で組み立てる（LoadConfig の事前条件）。リンク・FIFO・サイズは本関数が防ぐ。
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	fi, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !fi.Mode().IsRegular() {
		return nil, fmt.Errorf("%s: %w", path, errNotRegular)
	}
	b, err := io.ReadAll(io.LimitReader(f, maxSmallFileBytes+1))
	if err != nil {
		return nil, err
	}
	if len(b) > maxSmallFileBytes {
		return nil, fmt.Errorf("%s: %w（上限 %d バイト）", path, errTooLarge, maxSmallFileBytes)
	}
	return b, nil
}

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
	b, err := readSmallFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("%s: .runner の読み込みに失敗しました: %w", dir, err)
	}
	cfg, err := parseConfig(b)
	if err != nil {
		return Config{}, fmt.Errorf("%s: %w", dir, err)
	}
	return cfg, nil
}

// readVersion は <dir>/bin/runnerversion を読む。無い・読めない場合は空文字を返す。
func readVersion(dir string) string {
	b, err := readSmallFile(filepath.Join(dir, "bin", "runnerversion"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// readUnitName は <dir>/.service を読む。svc.sh install が systemd ユニット名を
// ここに書き出すため、ディレクトリとユニットを確実に紐付けられる。
// サービス化されていない・読めない場合は空文字を返す。
func readUnitName(dir string) string {
	b, err := readSmallFile(filepath.Join(dir, ".service"))
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
