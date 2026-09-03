package buildconfig

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// installModule は make install が起動できるバイナリを作ることを確かめるための最小の
// エントリポイント。Makefile の CMD（./cmd/gsr-helper）と同じ位置に置く。
var installModule = map[string]string{
	"cmd/gsr-helper/main.go": `package main

import "fmt"

func main() {
	fmt.Println("fixture ok")
}
`,
}

// make install はインストール先（GOBIN、無ければ GOPATH/bin）へ gsr という名前で
// 起動できるバイナリを置き、インストール先が PATH に無ければ案内を出す。make uninstall は
// それを消す。名前が gsr でなくなれば、ユーザーは gsr と打って起動できない。
func TestMakeInstallPutsRunnableGSRInInstallDir(t *testing.T) {
	dir := newModule(t, installModule)
	bin := filepath.Join(dir, "bin")
	env := append(slices.Clone(goWorkOff), "GOBIN="+bin)

	out, code := runMake(t, dir, env, "install")
	if code != 0 {
		t.Fatalf("make install が失敗した: exit=%d\n出力:\n%s", code, out)
	}

	// インストール先の一時ディレクトリは PATH に無いので、案内が出ていなければならない。
	// 案内が無いと、インストールできたのに gsr と打てない理由が利用者に分からない。
	if !strings.Contains(out, bin) {
		t.Errorf("PATH に無いインストール先の案内が出ていない\n出力:\n%s", out)
	}

	installed := filepath.Join(bin, "gsr")
	if _, err := os.Stat(installed); err != nil {
		t.Fatalf("インストール先に gsr が無い: %v\n出力:\n%s", err, out)
	}
	runOut, err := exec.Command(installed).CombinedOutput()
	if err != nil {
		t.Fatalf("インストールした gsr を起動できない: %v\n出力:\n%s", err, runOut)
	}
	if !strings.Contains(string(runOut), "fixture ok") {
		t.Errorf("インストールした gsr の出力が想定と違う\n出力:\n%s", runOut)
	}

	out, code = runMake(t, dir, env, "uninstall")
	if code != 0 {
		t.Fatalf("make uninstall が失敗した: exit=%d\n出力:\n%s", code, out)
	}
	if _, err := os.Stat(installed); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("make uninstall の後も gsr が残っている（%v）\n出力:\n%s", err, out)
	}
}
