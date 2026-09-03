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
	// インストール先の解決は 2 つあるので、両方を踏む。GOBIN を置く側だけを見ていると
	// GOPATH/bin へ落ちる分岐を一度も踏まず、INSTALL_DIR から GOPATH の項を丸ごと
	// 削っても緑のまま通る。
	tests := []struct {
		name string
		// resolve は base の下にインストール先を決め、その位置と渡す環境変数を返す。
		resolve func(base string) (installDir string, env []string)
	}{
		{
			name: "GOBIN",
			resolve: func(base string) (string, []string) {
				bin := filepath.Join(base, "gobin")
				return bin, []string{"GOBIN=" + bin}
			},
		},
		{
			name: "GOBIN が空なら GOPATH/bin",
			resolve: func(base string) (string, []string) {
				return filepath.Join(base, "bin"), []string{"GOBIN=", "GOPATH=" + base}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testMakeInstallPutsRunnableGSRIn(t, tt.resolve)
		})
	}
}

func testMakeInstallPutsRunnableGSRIn(t *testing.T, resolve func(base string) (string, []string)) {
	t.Helper()

	dir := newModule(t, installModule)
	bin, installEnv := resolve(t.TempDir())
	// GOENV=off を渡すのは、go env -w が書いた env ファイル（GOENV が指す）に GOBIN や
	// GOPATH が入っている環境（Nix・CI イメージ・direnv 等）では、環境変数へ空を渡しても
	// go env がファイル側の値を返してしまい、狙った分岐を踏めなくなるためである。
	env := append(slices.Clone(goWorkOff), "GOENV=off")
	env = append(env, installEnv...)

	out, code := runMake(t, dir, env, "install")
	if code != 0 {
		t.Fatalf("make install が失敗した: exit=%d\n出力:\n%s", code, out)
	}

	// インストール先の一時ディレクトリは PATH に無いので、案内が出ていなければならない。
	// 案内が無いと、インストールできたのに gsr と打てない理由が利用者に分からない。
	// インストール先のパスだけを見ると、案内より前に出るビルドコマンドの表示に当たって
	// 無条件に真になる（案内を丸ごと削っても緑になる）ので、案内文そのものを見る。
	for _, want := range []string{bin, "PATH にありません", "PATH に追加すると"} {
		if !strings.Contains(out, want) {
			t.Errorf("PATH に無いインストール先の案内に %q が無い\n出力:\n%s", want, out)
		}
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

// インストール先が PATH にあるときは、PATH への追加を促す案内を出してはならない。案内が
// 出る側だけを見ていると、案内文を常に出す実装でもテストが通ってしまう。
func TestMakeInstallOmitsPATHNoticeWhenInstallDirIsOnPATH(t *testing.T) {
	dir := newModule(t, installModule)
	bin := filepath.Join(dir, "bin")
	env := append(slices.Clone(goWorkOff),
		"GOBIN="+bin,
		"PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"),
	)

	out, code := runMake(t, dir, env, "install")
	if code != 0 {
		t.Fatalf("make install が失敗した: exit=%d\n出力:\n%s", code, out)
	}
	if strings.Contains(out, "PATH にありません") {
		t.Errorf("PATH にあるインストール先へ PATH 追加の案内が出ている\n出力:\n%s", out)
	}
	if !strings.Contains(out, "gsr と打って起動できます") {
		t.Errorf("gsr と打って起動できる旨の表示が無い\n出力:\n%s", out)
	}
}

// GOBIN も GOPATH も空ならインストール先は決められないので、install / uninstall は何もせず
// 理由を告げて失敗しなければならない。GOPATH が空のときに裸の /bin へ落ちると、sudo の要る
// system ディレクトリへ書き込もうとし、root では /bin/gsr を作って消してしまう。
func TestMakeInstallFailsWhenInstallDirIsUnresolvable(t *testing.T) {
	dir := newModule(t, installModule)
	// HOME も空にする。go env GOPATH は GOPATH が空なら $HOME/go へ落ちるため、
	// GOPATH だけ空にしても空にはならない。GOENV=off も要る——go env は空の環境変数を
	// 無視して env ファイル側の値を返すので、go env -w GOPATH=... を書いた環境では
	// GOPATH が空にならず、開発者の $GOPATH/bin/gsr を上書きして消してしまう。
	env := append(slices.Clone(goWorkOff), "GOENV=off", "GOBIN=", "GOPATH=", "HOME=")

	for _, target := range []string{"install", "uninstall"} {
		out, code := runMake(t, dir, env, target)
		if code == 0 {
			t.Errorf("make %s がインストール先を決められないのに成功した\n出力:\n%s", target, out)
		}
		if !strings.Contains(out, "インストール先を決められません") {
			t.Errorf("make %s がインストール先を決められない理由を告げていない\n出力:\n%s", target, out)
		}
		// ガードの後に続く実行（ビルドと削除）へ進んでいないことを見る。
		for _, forbidden := range []string{"build -o", "rm -f"} {
			if strings.Contains(out, forbidden) {
				t.Errorf("make %s がガードを抜けて %q を実行しようとした\n出力:\n%s", target, forbidden, out)
			}
		}
	}
}
