package buildconfig

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/ousiassllc/gsr-helper/internal/buildconfig/buildconfigtest"
)

type linterlyConfig struct {
	Rules struct {
		MaxLinesPerFile      *int `yaml:"max_lines_per_file"`
		MaxLinesPerDirectory *int `yaml:"max_lines_per_directory"`
		WarningThreshold     *int `yaml:"warning_threshold"`
	} `yaml:"rules"`
	CountMode   string `yaml:"count_mode"`
	UpdateCheck *bool  `yaml:"update_check"`
}

func loadLinterlyConfig(t *testing.T) linterlyConfig {
	t.Helper()

	raw, err := os.ReadFile(filepath.Join(buildconfigtest.RepoRoot(t), ".linterly.yml"))
	if err != nil {
		t.Fatalf(".linterly.yml を読めない: %v", err)
	}
	var cfg linterlyConfig
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		t.Fatalf(".linterly.yml を解析できない: %v", err)
	}
	return cfg
}

// 更新チェックは無効にする。既定は true で、実行のたびに GitHub Releases への
// 外向き HTTP が出る（self-hosted runner の不要な egress と CI ログへの混入）。
func TestLinterlyDisablesUpdateCheck(t *testing.T) {
	got := loadLinterlyConfig(t).UpdateCheck

	if got == nil {
		t.Fatal(".linterly.yml に update_check が無い（既定の true になる）")
	}
	if *got {
		t.Error("update_check が true になっている")
	}
}

// 行数上限は linterly の既定値のまま使う（上限に当たったら数値を上げるのではなく分割する）。
// あわせて `count_mode: all` と `warning_threshold` が存在することも見る——`rules:` が
// 空になると `rules section is required` で exit 2 になるため、既定値と同じ値でも消せない。
func TestLinterlyKeepsDefaultLineLimits(t *testing.T) {
	cfg := loadLinterlyConfig(t)

	if cfg.Rules.MaxLinesPerFile != nil {
		t.Errorf("max_lines_per_file が %d に上書きされている", *cfg.Rules.MaxLinesPerFile)
	}
	if cfg.Rules.MaxLinesPerDirectory != nil {
		t.Errorf("max_lines_per_directory が %d に上書きされている", *cfg.Rules.MaxLinesPerDirectory)
	}
	if cfg.Rules.WarningThreshold == nil {
		t.Error("warning_threshold が無い（rules: が空になると rules section is required で exit 2 になる）")
	}
	if cfg.CountMode != "all" {
		t.Errorf("count_mode が %q（期待: \"all\"）", cfg.CountMode)
	}
}
