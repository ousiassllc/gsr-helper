package check_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/audit"
	"github.com/ousiassllc/gsr-helper/internal/doctor/check"
	"github.com/ousiassllc/gsr-helper/internal/exec/command"
)

// secret は診断のコマンドが出力しうる権限情報を模した文字列。
//
// `sudo -l -U <user>` の出力はそのユーザーに許された全コマンドの一覧であり、
// 内部のデプロイスクリプトのパスなどが並ぶ。
const secret = "(root) NOPASSWD: /usr/local/bin/internal-deploy"

// 診断のコマンドの**出力**は監査ログに残らない。
//
// security.md「パスワード不要 sudo の要求への対応」は、`sudo -l -U` の出力は
// 権限情報であるため実行事実と終了コードだけを記録すると定めている。同書の
// 「監査ログ」はこの規則を全コマンドへ広げてある。
//
// **この保証は audit.Record が出力の欄を持たないことに由来する。** 記録する側に
// 出力を渡す口が無いので、呼び出し側が忘れても漏れない。回帰テストをここに置くのは、
// 将来 Record に stdout の欄が足された瞬間に落とすためである（doctor は SkipAudit を
// 立てずに `sudo -l -U` を通す唯一の経路であり、影響を最初に受ける）。
func TestProbeDoesNotRecordCommandOutput(t *testing.T) {
	t.Parallel()

	// 秘密は**引数ではなく出力**に現れる形にする。`sudo -l -U runner` の引数は
	// ユーザー名だけで、権限の一覧は標準出力へ出る。引数に置くと、引数を記録する
	// 仕様（マスク済みのコマンド行は残す）と区別が付かない検査になる。
	script := writeScript(t, secret)

	var buf bytes.Buffer
	lg := audit.New(&buf)
	ex := command.New(func() []string { return nil }, command.WithAudit(lg))

	in := check.Input{Exec: ex}
	res, err := in.Probe(context.Background(), "doctor.sudo", script)
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}
	// 出力そのものは呼び出し側には届いている（判定に使う）。
	if !strings.Contains(string(res.Stdout), "NOPASSWD") {
		t.Fatalf("標準出力 = %q, want 出力が返ること", res.Stdout)
	}

	line := buf.String()
	if line == "" {
		t.Fatal("監査ログに 1 行も記録されていない（実行の事実は残すこと）")
	}
	// 実行の事実と操作の識別子は残る。
	for _, want := range []string{"doctor.sudo", script} {
		if !strings.Contains(line, want) {
			t.Errorf("監査ログに %q が無い: %s", want, line)
		}
	}
	// 出力は残らない。
	for _, leak := range []string{"NOPASSWD", "internal-deploy"} {
		if strings.Contains(line, leak) {
			t.Errorf("コマンドの出力が監査ログに残っている（%q）: %s", leak, line)
		}
	}
}

// writeScript は本文を標準出力へ出すだけの実行可能なスクリプトを作り、そのパスを返す。
func writeScript(t *testing.T, body string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "probe.sh")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nprintf '%s\\n' \""+body+"\"\n"), 0o700); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return path
}

// 診断のコマンドは記録対象である（SkipAudit を立てない）。
//
// 同じ journalctl でも、ログ追従（internal/logs の 2 秒ごとの journalctl -u）は
// 他のレコードを押し流すため記録対象外にしてある。doctor の journalctl -k は
// 診断 1 回につき 1 本なので押し流しは起きない。記録対象かどうかはコマンド名では
// なく発行元で決まる（security.md「記録対象外とする読み取りコマンド」）。
func TestProbeIsRecorded(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	ex := command.New(func() []string { return nil }, command.WithAudit(audit.New(&buf)))

	in := check.Input{Exec: ex}
	if _, err := in.Probe(context.Background(), "doctor.oom", "true"); err != nil {
		t.Fatalf("Probe: %v", err)
	}
	if !strings.Contains(buf.String(), "doctor.oom") {
		t.Errorf("診断のコマンドが記録されていない: %s", buf.String())
	}
}
