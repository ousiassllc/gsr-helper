package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/audit"
)

// 監査ログには検証済みの SUDO_USER が記録される。
//
// audit の既定は SUDO_USER を検証せずに読むため、cmd が appconfig の検証済みの値を
// 渡していることをここで固定する。
func TestOpenAuditRecordsValidatedSudoUser(t *testing.T) {
	t.Setenv("SUDO_USER", "tester")
	path := filepath.Join(t.TempDir(), "audit.jsonl")

	var errOut strings.Builder
	lg := openAudit(path, &errOut)
	if errOut.Len() != 0 {
		t.Fatalf("警告が出ている: %s", errOut.String())
	}
	if err := lg.Write(audit.Record{Action: "test", Command: []string{"true"}}); err != nil {
		t.Fatalf("記録に失敗した: %v", err)
	}
	if err := lg.Close(); err != nil {
		t.Fatalf("クローズに失敗した: %v", err)
	}

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("監査ログを読めない: %v", err)
	}
	if !strings.Contains(string(b), `"sudo_user":"tester"`) {
		t.Errorf("監査ログ = %s, 検証済みの sudo_user が入っていない", b)
	}
}

// 文字種が不正な SUDO_USER は記録に残さない（検証済みの値を渡している証拠）。
func TestOpenAuditRejectsInvalidSudoUser(t *testing.T) {
	t.Setenv("SUDO_USER", "-oProxyCommand=evil")
	path := filepath.Join(t.TempDir(), "audit.jsonl")

	var errOut strings.Builder
	lg := openAudit(path, &errOut)
	if err := lg.Write(audit.Record{Action: "test", Command: []string{"true"}}); err != nil {
		t.Fatalf("記録に失敗した: %v", err)
	}
	_ = lg.Close()

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("監査ログを読めない: %v", err)
	}
	if strings.Contains(string(b), "evil") {
		t.Errorf("監査ログ = %s, 検証していない SUDO_USER を記録している", b)
	}
}

// 監査ログを開けなくても起動は続け、警告だけを出す。
func TestOpenAuditFallsBackToDiscard(t *testing.T) {
	// 通常ファイルの配下にはディレクトリを作れないため、必ず失敗する。
	file := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatalf("一時ファイルを作れない: %v", err)
	}

	var errOut strings.Builder
	lg := openAudit(filepath.Join(file, "audit.jsonl"), &errOut)
	if lg == nil {
		t.Fatal("記録先が nil になっている（呼び出し側に nil 判定を書かせない）")
	}
	if !strings.Contains(errOut.String(), "警告") {
		t.Errorf("標準エラー出力 = %q, 警告を出していない", errOut.String())
	}
	if err := lg.Write(audit.Record{Action: "test", Command: []string{"true"}}); err != nil {
		t.Errorf("記録先が Discard になっていない: %v", err)
	}
}

// ホスト名は取得できなくても空文字を返し、起動を止めない。
func TestHostname(t *testing.T) {
	if got := hostname(); strings.ContainsAny(got, " \t\n") {
		t.Errorf("ホスト名 = %q, 空白を含んでいる", got)
	}
}

// バージョン表記はビルド情報から解決できる（ldflags 未指定の経路）。
func TestVersionString(t *testing.T) {
	if got := versionString(); !strings.HasPrefix(got, appName+" ") {
		t.Errorf("バージョン表記 = %q, ツール名で始まっていない", got)
	}
}
