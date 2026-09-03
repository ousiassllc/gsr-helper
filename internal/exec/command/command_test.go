package command

import (
	"context"
	"errors"
	"os"
	osexec "os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/exec"
	"github.com/ousiassllc/gsr-helper/internal/exec/mask"
)

func TestCommandRunSuccess(t *testing.T) {
	name, args := helperCommand()
	ctx := exec.WithOptions(context.Background(), exec.Options{
		Action: "test.ok",
		Env:    helperEnv(helperStdoutEnv+"=out-ok", helperStderrEnv+"=err-ok"),
	})

	res, err := New(NoSecrets).Run(ctx, name, args...)
	if err != nil {
		t.Fatalf("Run がエラーを返した: %v", err)
	}
	if got := string(res.Stdout); got != "out-ok" {
		t.Errorf("Stdout = %q, want %q", got, "out-ok")
	}
	if got := string(res.Stderr); got != "err-ok" {
		t.Errorf("Stderr = %q, want %q", got, "err-ok")
	}
	if res.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0", res.ExitCode)
	}
}

func TestCommandRunNonZeroExit(t *testing.T) {
	const secret = "supersecrettoken"

	name, args := helperCommand("--token", secret)
	ctx := exec.WithOptions(context.Background(), exec.Options{
		Action: "test.fail",
		Env:    helperEnv(helperExitEnv+"=3", helperStderrEnv+"=だめでした"),
	})

	res, err := New(func() []string { return []string{secret} }).Run(ctx, name, args...)
	if res.ExitCode != 3 {
		t.Errorf("ExitCode = %d, want 3", res.ExitCode)
	}
	if got := string(res.Stderr); got != "だめでした" {
		t.Errorf("Stderr = %q, want %q", got, "だめでした")
	}

	var exitErr *ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("*ExitError が返っていない: %v", err)
	}
	if exitErr.Code != 3 {
		t.Errorf("ExitError.Code = %d, want 3", exitErr.Code)
	}
	if strings.Contains(strings.Join(exitErr.Args, " "), secret) {
		t.Errorf("ExitError.Args にトークンが残っている: %v", exitErr.Args)
	}
	if exitErr.Args[len(exitErr.Args)-1] != mask.Placeholder {
		t.Errorf("--token の値がマスクされていない: %v", exitErr.Args)
	}
	if strings.Contains(exitErr.Error(), secret) {
		t.Errorf("エラー文にトークンが残っている: %s", exitErr.Error())
	}
}

func TestCommandRunStartFailure(t *testing.T) {
	res, err := New(NoSecrets).Run(context.Background(), "gsr-helper-no-such-command")
	if err == nil {
		t.Fatal("起動できないコマンドでエラーを返していない")
	}
	if res.ExitCode != -1 {
		t.Errorf("ExitCode = %d, want -1", res.ExitCode)
	}

	var exitErr *ExitError
	if errors.As(err, &exitErr) {
		t.Error("起動失敗が *ExitError として返っている（非ゼロ終了と区別できない）")
	}
}

func TestCommandRunShellMetacharactersAreNotInterpreted(t *testing.T) {
	// シェルを経由しないため、メタ文字はコマンド名の一部として扱われ起動に失敗する。
	for _, name := range []string{"echo hi; ls", "true | false"} {
		t.Run(name, func(t *testing.T) {
			res, err := New(NoSecrets).Run(context.Background(), name)
			if err == nil {
				t.Fatalf("%q が実行されてしまった", name)
			}
			if res.ExitCode != -1 {
				t.Errorf("ExitCode = %d, want -1", res.ExitCode)
			}
		})
	}
}

func TestCommandRunUsesDir(t *testing.T) {
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("EvalSymlinks に失敗した: %v", err)
	}

	name, args := helperCommand()
	ctx := exec.WithOptions(context.Background(), exec.Options{
		Dir: dir,
		Env: helperEnv(helperModeEnv + "=cwd"),
	})

	res, err := New(NoSecrets).Run(ctx, name, args...)
	if err != nil {
		t.Fatalf("Run がエラーを返した: %v", err)
	}
	if got := string(res.Stdout); got != dir {
		t.Errorf("作業ディレクトリ = %q, want %q", got, dir)
	}
}

func TestCommandRunRelativeNameRequiresDir(t *testing.T) {
	res, err := New(NoSecrets).Run(context.Background(), "./config.sh", "--unattended")
	if err == nil {
		t.Fatal("Dir 未指定の相対パスコマンドでエラーを返していない")
	}
	if res.ExitCode != -1 {
		t.Errorf("ExitCode = %d, want -1", res.ExitCode)
	}
	if !strings.Contains(err.Error(), "作業ディレクトリ") {
		t.Errorf("理由が分かるエラーになっていない: %v", err)
	}
}

func TestCommandRunRelativeNameResolvesAgainstDir(t *testing.T) {
	// runner ディレクトリでの ./config.sh 実行が成り立つことを確かめる。
	// 相対パスはツール自身の cwd ではなく Dir を基準に解決される。
	dir := t.TempDir()
	if err := os.Symlink(os.Args[0], filepath.Join(dir, "helper")); err != nil {
		t.Fatalf("ヘルパーへのリンク作成に失敗した: %v", err)
	}

	_, args := helperCommand()
	ctx := exec.WithOptions(context.Background(), exec.Options{
		Dir: dir,
		Env: helperEnv(helperStdoutEnv + "=rel-ok"),
	})

	res, err := New(NoSecrets).Run(ctx, "./helper", args...)
	if err != nil {
		t.Fatalf("Run がエラーを返した: %v", err)
	}
	if got := string(res.Stdout); got != "rel-ok" {
		t.Errorf("Stdout = %q, want %q", got, "rel-ok")
	}
}

func TestCommandRunEnv(t *testing.T) {
	t.Setenv("GSR_HELPER_TEST_INHERITED", "inherited-value")

	tests := []struct {
		name string
		env  []string
		want string
	}{
		{
			name: "追加した環境変数が子に見える",
			env:  helperEnv(helperModeEnv+"=env", helperEchoEnv+"=GSR_HELPER_TEST_ADDED", "GSR_HELPER_TEST_ADDED=added-value"),
			want: "added-value",
		},
		{
			name: "既存の環境も継承する",
			env:  helperEnv(helperModeEnv+"=env", helperEchoEnv+"=GSR_HELPER_TEST_INHERITED"),
			want: "inherited-value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name, args := helperCommand()
			ctx := exec.WithOptions(context.Background(), exec.Options{Env: tt.env})

			res, err := New(NoSecrets).Run(ctx, name, args...)
			if err != nil {
				t.Fatalf("Run がエラーを返した: %v", err)
			}
			if got := string(res.Stdout); got != tt.want {
				t.Errorf("子が見た値 = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCommandRunInvalidEnv(t *testing.T) {
	for _, env := range []string{"BROKEN", "=VALUE"} {
		t.Run(env, func(t *testing.T) {
			name, args := helperCommand()
			ctx := exec.WithOptions(context.Background(), exec.Options{Env: []string{env}})

			res, err := New(NoSecrets).Run(ctx, name, args...)
			if err == nil {
				t.Fatalf("%q を受け付けてしまった", env)
			}
			if res.ExitCode != -1 {
				t.Errorf("ExitCode = %d, want -1", res.ExitCode)
			}
		})
	}
}

func TestCommandRunStdinIsClosed(t *testing.T) {
	name, args := helperCommand()
	ctx := exec.WithOptions(context.Background(), exec.Options{Env: helperEnv(helperModeEnv + "=stdin")})

	// 標準入力を与えないため、対話的なコマンドはハングせず即 EOF を受け取る。
	res, err := New(NoSecrets).Run(ctx, name, args...)
	if err != nil {
		t.Fatalf("Run がエラーを返した: %v", err)
	}
	if got := string(res.Stdout); got != "stdin=0" {
		t.Errorf("Stdout = %q, want %q", got, "stdin=0")
	}
}

func TestExitErrorMessage(t *testing.T) {
	e := &ExitError{Name: "systemctl", Args: []string{"stop", "x.service"}, Code: 5, Stderr: " failed\n"}
	want := "systemctl stop x.service が終了コード 5 で失敗しました: failed"
	if got := e.Error(); got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}

	noStderr := &ExitError{Name: "systemctl", Code: 5}
	if got := noStderr.Error(); got != "systemctl が終了コード 5 で失敗しました" {
		t.Errorf("Error() = %q", got)
	}
}

func TestExitErrorUnwrapsOSExitError(t *testing.T) {
	name, args := helperCommand()
	ctx := exec.WithOptions(context.Background(), exec.Options{Env: helperEnv(helperExitEnv + "=3")})

	_, err := New(NoSecrets).Run(ctx, name, args...)
	// 呼び出し側が os/exec のエラーまで辿れるようにする。
	var osExitErr *osexec.ExitError
	if !errors.As(err, &osExitErr) {
		t.Fatalf("*osexec.ExitError に到達できない: %v", err)
	}
	if osExitErr.ExitCode() != 3 {
		t.Errorf("ExitCode() = %d, want 3", osExitErr.ExitCode())
	}
}
