package edit_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/runner"
	"github.com/ousiassllc/gsr-helper/internal/runner/scope"
)

// sample は .env を持つ runner を 1 台、一時ディレクトリに用意する。
func sample(t *testing.T, name, env string) runner.Runner {
	t.Helper()

	dir := filepath.Join(t.TempDir(), name)
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatalf("runner ディレクトリの作成に失敗: %v", err)
	}
	if env != "" {
		write(t, filepath.Join(dir, ".env"), env)
	}

	return runner.Runner{
		Dir:      dir,
		Config:   runner.Config{AgentName: name},
		Scope:    scope.Scope{Kind: scope.Org, Owner: "foo"},
		UnitName: "actions.runner.foo." + name + ".service",
	}
}

// write は path へ内容を置く。
func write(t *testing.T, path, body string) {
	t.Helper()

	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("%s の作成に失敗: %v", path, err)
	}
}

// read は path の内容を返す。
func read(t *testing.T, path string) string {
	t.Helper()

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%s の読み込みに失敗: %v", path, err)
	}
	return string(b)
}
