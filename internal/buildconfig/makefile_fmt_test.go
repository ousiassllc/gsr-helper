package buildconfig

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/buildconfig/buildconfigtest"
)

// unformattedGo は gofmt が必ず整形し直すソース。
const unformattedGo = "package fixture\n\nfunc  Bad( ) {}\n"

// goWorkOff は一時モジュールが呼び出し元の go.work に巻き込まれないようにする。
var goWorkOff = []string{"GOWORK=off"}

// make fmt-check は入れ子の git worktree（.claude/worktrees/ 配下）を対象にしない。
// リポジトリルート自体が Go パッケージになっても、対象がディレクトリではなくファイル
// 単位で解決されるため gofmt がファイルシステムを再帰しない。
func TestFmtCheckSkipsNestedWorktree(t *testing.T) {
	dir := newModule(t, map[string]string{
		"root.go":                        "package fixture\n",
		".claude/worktrees/other/bad.go": unformattedGo,
	})

	out, code := runMake(t, dir, goWorkOff, "fmt-check")
	if code != 0 {
		t.Fatalf("入れ子 worktree 配下の未整形ファイルで fmt-check が失敗した: exit=%d\n出力:\n%s", code, out)
	}
}

// make fmt-check は testdata/ を対象にしない。go fmt ./... も testdata/ を対象外に
// するため、対象がずれると make fmt で直せないのに fmt-check が落ちる状態になる。
func TestFmtCheckSkipsTestdata(t *testing.T) {
	dir := newModule(t, map[string]string{
		"root.go":         "package fixture\n",
		"testdata/bad.go": unformattedGo,
	})

	out, code := runMake(t, dir, goWorkOff, "fmt-check")
	if code != 0 {
		t.Fatalf("testdata/ の未整形ファイルで fmt-check が失敗した: exit=%d\n出力:\n%s", code, out)
	}
}

// make fmt-check はモジュール内の未整形ファイルを検出する。
//
// fmt-check に掛かる他の検査は「何を対象にするか」を確かめるものである——入れ子 worktree と
// testdata/ を外す、ビルドタグで除外されたファイルも見る、PATH ではなく GOROOT 由来の gofmt を
// 使う、前提が崩れたら黙って通らない。**そもそも未整形を落とせるか**を見ているのはこの 1 本だけで、
// 除外の条件を書き換えて対象がまるごと消えても、範囲側の検査はすべて緑のままになる。
//
// 検出しない fmt-check は CI でも pre-commit でも常に成功するので、壊れたことに気付く経路が
// 他に無い。未整形のまま入ったコードは、次に誰かが make fmt を掛けた差分に無関係な整形として
// 混ざり込む。
func TestFmtCheckDetectsUnformatted(t *testing.T) {
	dir := newModule(t, map[string]string{"root.go": unformattedGo})

	out, code := runMake(t, dir, goWorkOff, "fmt-check")
	if code == 0 {
		t.Fatalf("未整形ファイルがあるのに fmt-check が成功した\n出力:\n%s", out)
	}
	if !strings.Contains(out, "root.go") {
		t.Errorf("出力に未整形ファイル名が含まれていない\n出力:\n%s", out)
	}
}

// ビルドタグで除外されたファイルも make fmt / make fmt-check の双方が対象にする。
// 片方だけが対象にすると、整形しても検査が落ち続ける状態になる。
func TestFmtAndFmtCheckCoverBuildTaggedFiles(t *testing.T) {
	dir := newModule(t, map[string]string{
		"root.go":   "package fixture\n",
		"tagged.go": "//go:build never\n\n" + unformattedGo,
	})

	out, code := runMake(t, dir, goWorkOff, "fmt-check")
	if code == 0 {
		t.Fatalf("ビルドタグで除外された未整形ファイルを fmt-check が見逃した\n出力:\n%s", out)
	}

	if out, code := runMake(t, dir, goWorkOff, "fmt"); code != 0 {
		t.Fatalf("make fmt が失敗した: exit=%d\n出力:\n%s", code, out)
	}
	if out, code := runMake(t, dir, goWorkOff, "fmt-check"); code != 0 {
		t.Fatalf("make fmt の後も fmt-check が失敗する: exit=%d\n出力:\n%s", code, out)
	}
}

// make fmt-check は PATH 上の gofmt ではなく $(GO) env GOROOT 由来の gofmt を使う。
// PATH に常に成功する gofmt を置いても検査結果は変わらない。
func TestFmtCheckUsesGorootGofmt(t *testing.T) {
	shimDir := t.TempDir()
	shim := filepath.Join(shimDir, "gofmt")
	if err := os.WriteFile(shim, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("gofmt のシムを作成できない: %v", err)
	}

	dir := newModule(t, map[string]string{"root.go": unformattedGo})
	env := append([]string{"PATH=" + shimDir + string(os.PathListSeparator) + os.Getenv("PATH")}, goWorkOff...)

	out, code := runMake(t, dir, env, "fmt-check")
	if code == 0 {
		t.Fatalf("PATH 上の gofmt シムを使ってしまい fmt-check が成功した\n出力:\n%s", out)
	}

	recipe, code := runMake(t, buildconfigtest.RepoRoot(t), nil, "-n", "fmt-check")
	if code != 0 {
		t.Fatalf("make -n fmt-check が失敗した: exit=%d\n出力:\n%s", code, recipe)
	}
	want := filepath.Join(goEnv(t, "GOROOT"), "bin", "gofmt")
	if !strings.Contains(recipe, want) {
		t.Errorf("fmt-check のレシピが %s を使っていない\nレシピ:\n%s", want, recipe)
	}
}

// go list が失敗したとき make fmt-check は失敗する。終了ステータスを捨てて
// 検査ゲートが静かに通ることがあってはならない。
func TestFmtCheckFailsWhenGoListFails(t *testing.T) {
	dir := newModule(t, map[string]string{
		"go.mod":  "この行は go.mod として不正\n",
		"root.go": "package fixture\n",
	})

	out, code := runMake(t, dir, goWorkOff, "fmt-check")
	if code == 0 {
		t.Fatalf("go.mod が壊れているのに fmt-check が成功した\n出力:\n%s", out)
	}
}

// 対象ファイルが 0 件のとき make fmt-check はハングせず失敗する。gofmt を引数なしで
// 起動すると標準入力を読んで待ち続けるため、明示的に検出して終了する必要がある。
func TestFmtCheckFailsWhenNoGoFiles(t *testing.T) {
	dir := newModule(t, nil)

	out, code := runMake(t, dir, goWorkOff, "fmt-check")
	if code == 0 {
		t.Fatalf("対象の Go ファイルが無いのに fmt-check が成功した\n出力:\n%s", out)
	}
	if !strings.Contains(out, "対象の Go ファイルがありません") {
		t.Errorf("対象 0 件であることが出力されていない\n出力:\n%s", out)
	}
}
