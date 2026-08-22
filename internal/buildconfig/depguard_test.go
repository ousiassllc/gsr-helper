package buildconfig

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// deniedCharmPrefix は禁止する Charm v1 系のモジュールパス接頭辞。
const deniedCharmPrefix = "github.com/charmbracelet"

type depguardDeny struct {
	Pkg  string `yaml:"pkg"`
	Desc string `yaml:"desc"`
}

type depguardConfig struct {
	Linters struct {
		Enable   []string `yaml:"enable"`
		Settings struct {
			Depguard struct {
				Rules map[string]struct {
					Deny []depguardDeny `yaml:"deny"`
				} `yaml:"rules"`
			} `yaml:"depguard"`
		} `yaml:"settings"`
	} `yaml:"linters"`
}

// charmStubModule は github.com/charmbracelet/lipgloss を名乗るローカルスタブを
// replace で解決する一時モジュール。ネットワークに出ずに禁止パスを import できる。
var charmStubModule = map[string]string{
	"go.mod": `module example.com/fixture

go 1.24

require github.com/charmbracelet/lipgloss v1.1.0

replace github.com/charmbracelet/lipgloss => ./stub
`,
	"stub/go.mod": `module github.com/charmbracelet/lipgloss

go 1.24
`,
	"stub/lipgloss.go": `// Package lipgloss は検証用のスタブ。
package lipgloss

// Width は幅を返す。
func Width(s string) int { return len(s) }
`,
	"fixture.go": `// Package fixture は検証用。
package fixture

import "github.com/charmbracelet/lipgloss"

// Width は幅を返す。
func Width(s string) int { return lipgloss.Width(s) }
`,
}

// Charm は charm.land/<name>/v2 に揃える。v1 系のパスを禁止していないと、
// golangci-lint の推移依存として go.mod に残る v1 を誤って import できてしまう。
func TestGolangciDeniesCharmV1Paths(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(repoRoot(t), ".golangci.yml"))
	if err != nil {
		t.Fatalf(".golangci.yml を読めない: %v", err)
	}
	var cfg depguardConfig
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		t.Fatalf(".golangci.yml を解析できない: %v", err)
	}

	found := false
	for _, rule := range cfg.Linters.Settings.Depguard.Rules {
		for _, deny := range rule.Deny {
			if strings.HasPrefix(deniedCharmPrefix, deny.Pkg) && deny.Desc != "" {
				found = true
			}
		}
	}
	if !found {
		t.Errorf("depguard に %s を理由付きで禁止するルールが無い", deniedCharmPrefix)
	}
}

// 設定だけでなく、実際に禁止パスの import が落ちることを確認する。
func TestGolangciLintRejectsCharmV1Import(t *testing.T) {
	dir := newModule(t, charmStubModule)

	cmd := exec.Command(golangciLintBinary(t), "run", "--config", filepath.Join(repoRoot(t), ".golangci.yml"), "./...")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), goWorkOff...)
	out, err := cmd.CombinedOutput()

	if err == nil {
		t.Fatalf("禁止パスを import しているのに lint が成功した\n出力:\n%s", out)
	}
	if !strings.Contains(string(out), "depguard") {
		t.Errorf("depguard の指摘が出ていない\n出力:\n%s", out)
	}
}
