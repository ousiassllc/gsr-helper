package buildconfig

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// embeddedConfigFiles は docs/environment/setup.md がコードブロックとして
// そのまま載せている設定ファイル。
var embeddedConfigFiles = []string{
	"Makefile",
	".github/workflows/ci.yml",
	".github/dependabot.yml",
	".golangci.yml",
	".linterly.yml",
	".linterlyignore",
	"lefthook.yml",
}

// 仕様書のコードブロックと設定ファイルの実体は一致していなければならない。
// 片方だけを直すと、仕様書を読んで再現した環境が実体と食い違う。
func TestSetupDocEmbedsConfigFilesVerbatim(t *testing.T) {
	root := repoRoot(t)

	doc, err := os.ReadFile(filepath.Join(root, "docs", "environment", "setup.md"))
	if err != nil {
		t.Fatalf("setup.md を読めない: %v", err)
	}

	for _, name := range embeddedConfigFiles {
		body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(name)))
		if err != nil {
			t.Fatalf("%s を読めない: %v", name, err)
		}
		if !strings.Contains(string(doc), string(body)) {
			t.Errorf("setup.md のコードブロックが %s の実体と一致しない", name)
		}
	}
}
