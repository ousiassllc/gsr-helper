package buildconfig

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

type lefthookCommand struct {
	Run        string      `yaml:"run"`
	Glob       yamlStrings `yaml:"glob"`
	Priority   int         `yaml:"priority"`
	StageFixed bool        `yaml:"stage_fixed"`
}

type lefthookHook struct {
	Parallel bool                       `yaml:"parallel"`
	Commands map[string]lefthookCommand `yaml:"commands"`
}

type lefthookConfig struct {
	Lefthook  string       `yaml:"lefthook"`
	PreCommit lefthookHook `yaml:"pre-commit"`
	PrePush   lefthookHook `yaml:"pre-push"`
}

func loadLefthookConfig(t *testing.T) lefthookConfig {
	t.Helper()

	raw, err := os.ReadFile(filepath.Join(repoRoot(t), "lefthook.yml"))
	if err != nil {
		t.Fatalf("lefthook.yml を読めない: %v", err)
	}
	var cfg lefthookConfig
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		t.Fatalf("lefthook.yml を解析できない: %v", err)
	}
	return cfg
}

func lefthookCommandOf(t *testing.T, hook lefthookHook, name string) lefthookCommand {
	t.Helper()

	cmd, ok := hook.Commands[name]
	if !ok {
		t.Fatalf("lefthook.yml に command %q が無い", name)
	}
	return cmd
}

// hook 実行時のバージョンは go.mod に固定する。指定が無いと hook スクリプトの
// 探索順で PATH 上の lefthook が go tool lefthook より先に選ばれる。
func TestLefthookPinsVersionToGoTool(t *testing.T) {
	if got := loadLefthookConfig(t).Lefthook; got != "go tool lefthook" {
		t.Errorf("lefthook.yml の lefthook: が %q（期待: \"go tool lefthook\"）", got)
	}
}

// pre-commit の lint は HEAD からの差分だけを対象にする。作業ツリー全体を無条件に
// 検査すると、コミット済みの既存指摘 1 件で以後のコミットが落ち続ける。
func TestLefthookPreCommitLintIsScopedToDiff(t *testing.T) {
	run := lefthookCommandOf(t, loadLefthookConfig(t).PreCommit, "lint").Run
	if !strings.Contains(run, "--new-from-rev=HEAD") {
		t.Errorf("pre-commit の lint が差分に絞られていない: %q", run)
	}
}

// 対象を絞れない・絞る必要のないコマンドは make ターゲットを経由し、コマンド列の
// 二重管理を作らない。逆にスコープが必要なコマンドは make を経由しない。
func TestLefthookRoutesUnscopedCommandsThroughMake(t *testing.T) {
	cfg := loadLefthookConfig(t)

	tests := []struct {
		hookName string
		hook     lefthookHook
		command  string
		wantRun  string
	}{
		{"pre-commit", cfg.PreCommit, "linterly", "make linterly"},
		{"pre-push", cfg.PrePush, "test", "make test"},
	}
	for _, tt := range tests {
		if got := lefthookCommandOf(t, tt.hook, tt.command).Run; got != tt.wantRun {
			t.Errorf("%s の %s が %q（期待: %q）", tt.hookName, tt.command, got, tt.wantRun)
		}
	}

	for _, name := range []string{"fmt", "lint"} {
		run := lefthookCommandOf(t, cfg.PreCommit, name).Run
		if strings.Contains(run, "make ") {
			t.Errorf("スコープが必要な pre-commit の %s が make を経由している: %q", name, run)
		}
	}
}

// 実行順は priority で固定する。parallel: false は同時実行を止めるだけで順序を
// 決めないため、未指定だと commands のキー名の比較に依存してしまう。
func TestLefthookPreCommitOrderIsPinnedByPriority(t *testing.T) {
	pre := loadLefthookConfig(t).PreCommit

	if pre.Parallel {
		t.Error("pre-commit が parallel: true になっている（fmt の整形結果を lint が読めない）")
	}
	for name, cmd := range pre.Commands {
		if cmd.Priority <= 0 {
			t.Errorf("pre-commit の %s に priority が無い（実行順が命名依存になる）", name)
		}
	}

	// fmt が整形した結果を lint が読むため、この順序は逆転してはならない。
	order := []string{"fmt", "lint", "linterly"}
	for i := 1; i < len(order); i++ {
		prev := lefthookCommandOf(t, pre, order[i-1])
		next := lefthookCommandOf(t, pre, order[i])
		if prev.Priority >= next.Priority {
			t.Errorf("priority が %s(%d) >= %s(%d) になっている", order[i-1], prev.Priority, order[i], next.Priority)
		}
	}
}

// lefthook.yml は go.mod でピン留めしたバージョンの lefthook が受け付ける形でなければならない。
func TestLefthookConfigIsValid(t *testing.T) {
	cmd := exec.Command("go", "tool", "lefthook", "validate")
	cmd.Dir = repoRoot(t)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("lefthook validate が失敗した: %v\n出力:\n%s", err, out)
	}
}
