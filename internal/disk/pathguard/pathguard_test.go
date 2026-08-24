package pathguard_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/disk/pathguard"
)

// Validate は root 権限での誤削除を防ぐ最後の砦であり、異常系の網羅が
// 受け入れ条件になっている（docs/architecture/security.md
// 「削除パスの検証を必須にする」）。ここでは実際に一時ディレクトリと
// シンボリックリンクを作り、経路の逸脱が実物で弾かれることを確かめる。

// newValidateTree は検証用のディレクトリ構造を作り、runner ディレクトリと
// その外側のディレクトリを返す。
//
//	<root>/runner/_work/repo
//	<root>/runner/_work/_temp/x
//	<root>/runner/_work/escape -> <root>/outside
//	<root>/runner/_diag
//	<root>/runner/bin
//	<root>/outside
func newValidateTree(t *testing.T) (base, outside string) {
	t.Helper()

	root := t.TempDir()
	base = filepath.Join(root, "runner")
	outside = filepath.Join(root, "outside")
	for _, dir := range []string{
		filepath.Join(base, "_work", "repo"),
		filepath.Join(base, "_work", "_temp", "x"),
		filepath.Join(base, "_diag"),
		filepath.Join(base, "bin"),
		outside,
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("%s の作成に失敗した: %v", dir, err)
		}
	}
	// 許可サブツリーの中から基準の外へ出るリンク。検証はリンクを解決して弾く。
	if err := os.Symlink(outside, filepath.Join(base, "_work", "escape")); err != nil {
		t.Fatalf("シンボリックリンクの作成に失敗した: %v", err)
	}
	return base, outside
}

func TestValidateAccepts(t *testing.T) {
	base, _ := newValidateTree(t)

	tests := map[string]string{
		"_work 配下のリポジトリ": filepath.Join(base, "_work", "repo"),
		"_work 自身":       filepath.Join(base, "_work"),
		"_diag 自身":       filepath.Join(base, "_diag"),
		"_work/_temp 配下": filepath.Join(base, "_work", "_temp", "x"),
	}
	for name, target := range tests {
		t.Run(name, func(t *testing.T) {
			if err := pathguard.Validate(base, target); err != nil {
				t.Errorf("Validate(%q) がエラーを返した: %v", target, err)
			}
		})
	}
}

func TestValidateRejects(t *testing.T) {
	base, outside := newValidateTree(t)

	tests := []struct {
		name string
		base string
		// target は検証対象のパス。
		target string
		// wantMsg はエラー文言の一部。どの条件で落ちたかを読み分けるため、
		// 「何らかのエラー」ではなく理由まで固定する。
		wantMsg string
	}{
		{
			name:    ".. を含むパス",
			base:    base,
			target:  filepath.Join(base, "_work", "repo") + "/../../bin",
			wantMsg: `".." が含まれています`,
		},
		{
			name:    "基準ディレクトリ外への絶対パス",
			base:    base,
			target:  outside,
			wantMsg: "の配下にありません",
		},
		{
			name:    "シンボリックリンクで基準の外へ出るパス",
			base:    base,
			target:  filepath.Join(base, "_work", "escape"),
			wantMsg: "の配下にありません",
		},
		{
			name:    "基準ディレクトリ自身",
			base:    base,
			target:  base,
			wantMsg: "基準ディレクトリ自身は削除できません",
		},
		{
			name:    "許可サブツリーの外",
			base:    base,
			target:  filepath.Join(base, "bin"),
			wantMsg: "削除が許可されたサブツリー",
		},
		{
			name:    "存在しないパス",
			base:    base,
			target:  filepath.Join(base, "_work", "missing"),
			wantMsg: "削除対象のパスのシンボリックリンク解決に失敗しました",
		},
		{
			name:    "存在しない基準ディレクトリ",
			base:    filepath.Join(base, "missing"),
			target:  filepath.Join(base, "_work"),
			wantMsg: "基準ディレクトリのシンボリックリンク解決に失敗しました",
		},
		{
			name:    "基準ディレクトリが空",
			base:    "",
			target:  filepath.Join(base, "_work"),
			wantMsg: "基準ディレクトリが空です",
		},
		{
			name:    "対象パスが空",
			base:    base,
			target:  "",
			wantMsg: "削除対象のパスが空です",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := pathguard.Validate(tt.base, tt.target)
			if err == nil {
				t.Fatalf("Validate(%q, %q) が許可してしまった", tt.base, tt.target)
			}
			if !strings.Contains(err.Error(), tt.wantMsg) {
				t.Errorf("エラー文言 = %q, want %q を含む", err.Error(), tt.wantMsg)
			}
		})
	}
}

// TestValidateRejectsSymlinkedSubtree は _work 自体がリンクで外を指す runner を
// 弾くことを確かめる。リンクを解決せずにパスの文字列だけで判定していると、
// 基準ディレクトリ配下に見えるまま外部のツリーを消してしまう。
func TestValidateRejectsSymlinkedSubtree(t *testing.T) {
	root := t.TempDir()
	base := filepath.Join(root, "runner")
	outside := filepath.Join(root, "outside", "repo")
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatalf("外部ツリーの作成に失敗した: %v", err)
	}
	if err := os.MkdirAll(base, 0o755); err != nil {
		t.Fatalf("基準ディレクトリの作成に失敗した: %v", err)
	}
	if err := os.Symlink(filepath.Join(root, "outside"), filepath.Join(base, "_work")); err != nil {
		t.Fatalf("シンボリックリンクの作成に失敗した: %v", err)
	}

	target := filepath.Join(base, "_work", "repo")
	if err := pathguard.Validate(base, target); err == nil {
		t.Fatalf("リンクで外を指す _work 配下を許可してしまった: %s", target)
	}
}
