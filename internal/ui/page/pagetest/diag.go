package pagetest

import (
	"context"

	"fmt"
	"github.com/ousiassllc/gsr-helper/internal/doctor/check"
	"os"
	"path/filepath"
	"time"

	"github.com/ousiassllc/gsr-helper/internal/logs"
	"github.com/ousiassllc/gsr-helper/internal/runner"
)

// runner の `_diag` 配下（ログファイル）のフィクスチャを組み立てる。
//
// タブ 1 枚のためのフィクスチャをここに置くのは、`page/<tab>` のディレクトリが行数上限
// （1 ディレクトリ 2000 行）に近く、道具をこちらへ寄せる方針だからである（pagetest.go の
// doc と atomic-design.md の「ディレクトリの行数」）。ログを開く導線は Logs タブだけの
// ものではない（Runners / Jobs の `l`）ため、共有の置き場としても筋が通る。

// DiagRunner は dir を `_diag` の親に持つ runner を返す。
//
// SampleRunner の Dir は実在しないパスなので、ファイルを置く検証はこちらを使う。
func DiagRunner(name, dir string) runner.Runner {
	r := SampleRunner()
	r.Dir = dir
	r.Config.AgentName = name
	r.WorkDir = filepath.Join(dir, "_work")
	r.UnitName = "actions.runner.foo." + name + ".service"
	return r
}

// WriteDiagLog は runner の dir 配下の `_diag` にログを 1 つ作り、更新時刻を設定する。
//
// 更新時刻まで設定するのは、一覧の並びが更新時刻の降順で決まる（FR-23）ためである。
// 合否の判定は呼び出し側に残し、ここは失敗の理由を返すだけにする（run.go の doc）。
func WriteDiagLog(dir, name, body string, mod time.Time) (string, error) {
	diag := filepath.Join(dir, logs.DiagDir)
	if err := os.MkdirAll(diag, 0o750); err != nil {
		return "", fmt.Errorf("_diag を作れない: %w", err)
	}
	path := filepath.Join(diag, name)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		return "", fmt.Errorf("%s を書けない: %w", name, err)
	}
	if err := os.Chtimes(path, mod, mod); err != nil {
		return "", fmt.Errorf("%s の時刻を設定できない: %w", name, err)
	}
	return path, nil
}

// StubCheck は診断項目の差し替え。実ホストを見ずに固定の判定を返す。
//
// 起動時の前提チェック（FR-44）を通る経路を、親 Model からも page からも
// 実行環境の構成に依存せずに検証するための道具である。本物の項目は sudo /
// docker / /etc/group を読むため、そのままでは CI と手元で結果が変わる。
type StubCheck struct {
	// Status は返す判定。
	Status check.Status
}

// ID は固定の識別子を返す。
func (StubCheck) ID() string { return "job.stub" }

// Category はジョブ実行の前提を返す（起動時の対象はこの分類に限る）。
func (StubCheck) Category() string { return check.CatJobReq }

// Startup は真を返す。起動時に走る項目を模すための道具だからである。
func (StubCheck) Startup() bool { return true }

// Run は固定の判定を 1 件返す。
func (s StubCheck) Run(context.Context, check.Input) []check.Result {
	return []check.Result{{ID: s.ID(), Category: s.Category(), Status: s.Status, Startup: true}}
}
