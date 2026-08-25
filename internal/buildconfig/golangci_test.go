package buildconfig

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/ousiassllc/gsr-helper/internal/buildconfig/buildconfigtest"
)

type nolintlintSettings struct {
	AllowUnused        bool `yaml:"allow-unused"`
	RequireExplanation bool `yaml:"require-explanation"`
	RequireSpecific    bool `yaml:"require-specific"`
}

type gciSettings struct {
	Sections    []string `yaml:"sections"`
	CustomOrder *bool    `yaml:"custom-order"`
}

type golangciConfig struct {
	Linters struct {
		Enable   []string `yaml:"enable"`
		Settings struct {
			Nolintlint nolintlintSettings `yaml:"nolintlint"`
		} `yaml:"settings"`
	} `yaml:"linters"`
	Formatters struct {
		Enable   []string `yaml:"enable"`
		Settings struct {
			Gci gciSettings `yaml:"gci"`
		} `yaml:"settings"`
	} `yaml:"formatters"`
	Issues struct {
		MaxIssuesPerLinter *int `yaml:"max-issues-per-linter"`
		MaxSameIssues      *int `yaml:"max-same-issues"`
	} `yaml:"issues"`
}

func loadGolangciConfig(t *testing.T) golangciConfig {
	t.Helper()

	raw, err := os.ReadFile(filepath.Join(buildconfigtest.RepoRoot(t), ".golangci.yml"))
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
	cmd.Dir = buildconfigtest.RepoRoot(t)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("golangci-lint の実体を解決できない: %v", err)
	}
	return strings.TrimSpace(string(out))
}

// 抑制には理由コメントとリンター名が必須で、不要になった抑制も検出
// される。nolintlint を `require-explanation` / `require-specific` / `allow-unused: false` の 3 つ
// すべて有効で使う。
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

// golangci-lint が同種の指摘を打ち切らない（`max-issues-per-linter` と `max-same-issues` の
// どちらも `0` = 無制限を明示する）。既定の 50 / 3 のままだと、抑制やエラーの棚卸しで件数を
// 数え上げられない。
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

// 抑制の 3 つの取り決め（リンター名の明示・理由コメント・不要になった抑制）を破った `//nolint` が、
// リポジトリの `.golangci.yml` で実際に落ちる。
func TestGolangciLintRejectsSloppyNolint(t *testing.T) {
	bin := golangciLintBinary(t)
	config := filepath.Join(buildconfigtest.RepoRoot(t), ".golangci.yml")

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

// import は標準ライブラリ / 外部モジュール / 自前パッケージの 3 グループに固定される。gci を
// この順のセクションで使い、`custom-order: true` も検査する——これが無いと gci は記載順を無視して
// 内蔵の既定順で並べ、順序の取り決めが実効にならない。
func TestGolangciEnablesGci(t *testing.T) {
	cfg := loadGolangciConfig(t)

	if !slices.Contains(cfg.Formatters.Enable, "gci") {
		t.Fatalf("gci が有効になっていない: %v", cfg.Formatters.Enable)
	}
	want := []string{"standard", "default", "prefix(github.com/ousiassllc/gsr-helper)"}
	if got := cfg.Formatters.Settings.Gci.Sections; !slices.Equal(got, want) {
		t.Errorf("gci.sections が %v（期待: %v）", got, want)
	}
	// custom-order が無いと gci は sections の記載順を無視して内蔵の既定順で
	// 並べる（実測）。上の順序アサーションを実効にするために必須である。
	if got := cfg.Formatters.Settings.Gci.CustomOrder; got == nil || !*got {
		t.Errorf("gci.custom-order が true になっていない: %v", got)
	}
}

// 自前パッケージが標準ライブラリのグループに混ざった import が、リポジトリの `.golangci.yml` で
// 実際に落ちる。gofmt はグループ内を並べ替えるだけなので `make fmt-check` では検出できない。
//
// この死角は Issue #119 で実際に踏んでいる。フィクスチャの import はアルファベット順に
// 並んでいるので、指摘が出るとすれば gci だけである。
func TestGolangciLintRejectsMisgroupedImports(t *testing.T) {
	bin := golangciLintBinary(t)
	config := filepath.Join(buildconfigtest.RepoRoot(t), ".golangci.yml")

	// prefix セクションに当てるため、フィクスチャのモジュールパスを本リポジトリに合わせる。
	dir := newModule(t, map[string]string{
		"go.mod": "module github.com/ousiassllc/gsr-helper\n\ngo 1.24\n",
		"sub/sub.go": `// Package sub は検証用。
package sub

// Name は名前を返す。
func Name() string { return "sub" }
`,
		"fixture.go": `// Package fixture は検証用。
package fixture

import (
	"github.com/ousiassllc/gsr-helper/sub"
	"strings"
)

// Upper は sub の名前を大文字にする。
func Upper() string { return strings.ToUpper(sub.Name()) }
`,
	})

	cmd := exec.Command(bin, "run", "--config", config, "./...")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), goWorkOff...)
	out, err := cmd.CombinedOutput()

	if err == nil {
		t.Fatalf("自前パッケージが標準ライブラリのグループに混ざっているのに lint が成功した\n出力:\n%s", out)
	}
	// 部分一致で "gci" を探すと golangci-lint 自身の名前や
	// ".golangci.yml" にも当たり、設定を読めなかった場合や
	// 多重起動で弾かれた場合（どちらも非 0 終了）に gci が
	// フィクスチャを一度も評価していないのに通ってしまう。
	// gci の実出力そのもので判定する。
	const gciFinding = "File is not properly formatted (gci)"
	if !strings.Contains(string(out), gciFinding) {
		t.Errorf("gci の指摘が出ていない\n出力:\n%s", out)
	}
}
