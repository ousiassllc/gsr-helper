package buildconfig

import (
	"strings"
	"testing"
)

// racyModule は 2 つの goroutine が同じ変数へ同時に書き込むだけのモジュール。
// 競合検出が無効だとテストは成功してしまう。
var racyModule = map[string]string{
	"race.go": `package fixture

// Race は競合検出の確認用。
func Race() int {
	shared := 0
	done := make(chan struct{})
	go func() {
		shared++
		close(done)
	}()
	shared++
	<-done
	return shared
}
`,
	"race_test.go": `package fixture

import "testing"

func TestRace(t *testing.T) {
	if Race() == 0 {
		t.Fatal("到達しない")
	}
}
`,
}

// make test は競合検出付きで実行する。CI とローカルの唯一のテスト経路であり、
// -race が外れると並行処理の退行が緑のまま通過する。
func TestMakeTestDetectsDataRace(t *testing.T) {
	dir := newModule(t, racyModule)

	out, code := runMake(t, dir, goWorkOff, "test")
	if code == 0 {
		t.Fatalf("競合するテストなのに make test が成功した\n出力:\n%s", out)
	}
	if !strings.Contains(out, "DATA RACE") {
		t.Errorf("make test が競合検出（-race）で失敗していない\n出力:\n%s", out)
	}
}
