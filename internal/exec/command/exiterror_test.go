package command

import (
	"strings"
	"testing"
)

// 標準エラー出力が空なら標準出力を理由として出す。
//
// 理由を標準出力へ書いて終わるコマンドがある——runner 付属の config.sh は root で
// RUNNER_ALLOW_RUNASROOT が空のとき `Must not run with sudo` を echo して終了コード 1 で
// 終わる。**stderr だけを見ていると「終了コード 1 で失敗しました」しか出せず、利用者は
// 原因に辿り着けない**（実機で発生した）。
func TestExitErrorFallsBackToStdoutWhenStderrIsEmpty(t *testing.T) {
	t.Parallel()

	err := &ExitError{
		Name: "./config.sh", Args: []string{"--unattended"}, Code: 1,
		Stderr: "", Stdout: "Must not run with sudo\n", Err: nil,
	}

	if got := err.Error(); !strings.Contains(got, "Must not run with sudo") {
		t.Errorf("Error() = %q, want 標準出力の理由を含む", got)
	}
	// 監査ログに載る要約は出力を含めない（Summary の doc）。
	if got := err.Summary(); strings.Contains(got, "Must not run with sudo") {
		t.Errorf("Summary() = %q, want 出力を含まない", got)
	}
}

// 両方あるときは標準エラー出力を採る。失敗の理由はそちらに出るのが通例である。
func TestExitErrorPrefersStderrOverStdout(t *testing.T) {
	t.Parallel()

	err := &ExitError{
		Name: "./svc.sh", Args: nil, Code: 2,
		Stderr: "permission denied", Stdout: "starting", Err: nil,
	}

	got := err.Error()
	if !strings.Contains(got, "permission denied") || strings.Contains(got, "starting") {
		t.Errorf("Error() = %q, want 標準エラー出力のみ", got)
	}
}
