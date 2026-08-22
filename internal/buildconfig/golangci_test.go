package buildconfig

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

type nolintlintSettings struct {
	AllowUnused        bool `yaml:"allow-unused"`
	RequireExplanation bool `yaml:"require-explanation"`
	RequireSpecific    bool `yaml:"require-specific"`
}

type golangciConfig struct {
	Linters struct {
		Enable   []string `yaml:"enable"`
		Settings struct {
			Nolintlint nolintlintSettings `yaml:"nolintlint"`
		} `yaml:"settings"`
	} `yaml:"linters"`
	Issues struct {
		MaxIssuesPerLinter *int `yaml:"max-issues-per-linter"`
		MaxSameIssues      *int `yaml:"max-same-issues"`
	} `yaml:"issues"`
}

func loadGolangciConfig(t *testing.T) golangciConfig {
	t.Helper()

	raw, err := os.ReadFile(filepath.Join(repoRoot(t), ".golangci.yml"))
	if err != nil {
		t.Fatalf(".golangci.yml を読めない: %v", err)
	}
	var cfg golangciConfig
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		t.Fatalf(".golangci.yml を解析できない: %v", err)
	}
	return cfg
}

// golangciLintBinary は go.mod でピン留めした golangci-lint の実体パスを返す。
// 一時モジュールを対象に走らせるため、`go tool` 経由ではなく実体を使う。
func golangciLintBinary(t *testing.T) string {
	t.Helper()

	cmd := exec.Command("go", "tool", "-n", "golangci-lint")
	cmd.Dir = repoRoot(t)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("golangci-lint の実体を解決できない: %v", err)
	}
	return strings.TrimSpace(string(out))
}

// 抑制方針（行単位・理由必須・リンター名必須）を機械的に強制するため、
// nolintlint を 3 つの設定すべて有効で使う。
func TestGolangciEnablesNolintlint(t *testing.T) {
	cfg := loadGolangciConfig(t)

	if !slices.Contains(cfg.Linters.Enable, "nolintlint") {
		t.Fatalf("nolintlint が有効になっていない: %v", cfg.Linters.Enable)
	}
	got := cfg.Linters.Settings.Nolintlint
	want := nolintlintSettings{AllowUnused: false, RequireExplanation: true, RequireSpecific: true}
	if got != want {
		t.Errorf("nolintlint の設定が %+v（期待: %+v）", got, want)
	}
}

// 同種の指摘を打ち切らない。既定（3 / 50）のままだと抑制やエラーの棚卸しで
// 件数を数え上げられない。
func TestGolangciDoesNotTruncateIssues(t *testing.T) {
	cfg := loadGolangciConfig(t)

	tests := []struct {
		key   string
		value *int
	}{
		{"max-issues-per-linter", cfg.Issues.MaxIssuesPerLinter},
		{"max-same-issues", cfg.Issues.MaxSameIssues},
	}
	for _, tt := range tests {
		switch {
		case tt.value == nil:
			t.Errorf("issues.%s が設定されていない（既定で打ち切られる）", tt.key)
		case *tt.value != 0:
			t.Errorf("issues.%s が %d（期待: 0 = 無制限）", tt.key, *tt.value)
		}
	}
}

// nolintlint が実際に雑な抑制を落とすことを、リポジトリの設定で確認する。
func TestGolangciLintRejectsSloppyNolint(t *testing.T) {
	bin := golangciLintBinary(t)
	config := filepath.Join(repoRoot(t), ".golangci.yml")

	tests := []struct {
		name   string
		source string
		want   string
	}{
		{
			name: "リンター名のない抑制",
			source: `// Package fixture は検証用。
package fixture

import "os"

// Remove はファイルを消す。
func Remove(p string) {
	os.Remove(p) //nolint // 理由: 検証用
}
`,
			want: "should mention specific linter",
		},
		{
			name: "理由コメントのない抑制",
			source: `// Package fixture は検証用。
package fixture

import "os"

// Read はファイルを読む。
func Read(p string) ([]byte, error) {
	return os.ReadFile(p) //nolint:gosec
}
`,
			want: "should provide explanation",
		},
		{
			name: "不要になった抑制",
			source: `// Package fixture は検証用。
package fixture

// Clean は何もしない。
func Clean() { //nolint:errcheck // 理由: 抑制対象が無い
}
`,
			want: "is unused",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := newModule(t, map[string]string{"fixture.go": tt.source})

			cmd := exec.Command(bin, "run", "--config", config, "./...")
			cmd.Dir = dir
			cmd.Env = append(os.Environ(), goWorkOff...)
			out, err := cmd.CombinedOutput()

			if err == nil {
				t.Fatalf("雑な抑制なのに lint が成功した\n出力:\n%s", out)
			}
			if !strings.Contains(string(out), tt.want) {
				t.Errorf("nolintlint の指摘 %q が出ていない\n出力:\n%s", tt.want, out)
			}
		})
	}
}
