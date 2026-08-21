package runner

import (
	"os"
	"path/filepath"
	"sort"
)

// defaultDepth は走査ルート配下を掘る既定の深さ。
const defaultDepth = 2

// defaultRootGlobs は runner の一般的な設置場所。
// 誤検出と走査コストを抑えるため、広すぎるパターンは置かない。
var defaultRootGlobs = []string{
	"/home/*/actions-runner*",
	"/home/*/runners",
	"/root/actions-runner*",
	"/opt/actions-runner*",
	"/opt/runner*",
	"/opt/*/actions-runner*",
	"/srv/actions-runner*",
	"/srv/*/actions-runner*",
	"/var/lib/actions-runner*",
	"/usr/local/actions-runner*",
}

// DefaultRoots は既定の走査ルートを展開して返す。
func DefaultRoots() []string {
	var roots []string
	for _, g := range defaultRootGlobs {
		matches, err := filepath.Glob(g)
		if err != nil {
			continue // パターン不正のみ。実行時には起きない
		}
		roots = append(roots, matches...)
	}
	return roots
}

// collectDirs は走査・プロセス・ユニットの各経路から runner ディレクトリを集める。
func collectDirs(opts Options, procs []Process, units []SvcState) []string {
	depth := opts.Depth
	if depth <= 0 {
		depth = defaultDepth
	}

	seen := map[string]bool{}
	var dirs []string
	add := func(dir string) {
		dir = normalizeDir(dir)
		if dir == "" || seen[dir] || !IsRunnerDir(dir) {
			return
		}
		seen[dir] = true
		dirs = append(dirs, dir)
	}

	for _, root := range append(DefaultRoots(), opts.Roots...) {
		for _, d := range findRunnerDirs(root, depth) {
			add(d)
		}
	}
	for _, p := range procs {
		add(p.Dir)
	}
	for _, u := range units {
		add(u.WorkingDir)
	}

	sort.Strings(dirs)
	return dirs
}

// findRunnerDirs は root 配下から runner ディレクトリを探す。
// runner ディレクトリを見つけたらその配下は掘らない（_work が巨大になるため）。
func findRunnerDirs(root string, depth int) []string {
	if depth < 0 {
		// collectDirs が 0 以下を defaultDepth に寄せるため production 経路からは
		// 到達しない。再帰の打ち切り条件を呼び出し側の誤りから守るための防御。
		return nil
	}
	fi, err := os.Stat(root)
	if err != nil || !fi.IsDir() {
		return nil
	}
	if IsRunnerDir(root) {
		return []string{root}
	}
	if depth == 0 {
		return nil
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	var found []string
	for _, e := range entries {
		if !e.IsDir() || e.Name() == "_work" || e.Name() == "_diag" {
			continue
		}
		found = append(found, findRunnerDirs(filepath.Join(root, e.Name()), depth-1)...)
	}
	return found
}

// normalizeDir はディレクトリパスを比較可能な形に正規化する。
func normalizeDir(dir string) string {
	if dir == "" {
		return ""
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		abs = dir
	}
	// プロセスの cwd が削除済みの場合 " (deleted)" が付くため解決に失敗する。
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		return resolved
	}
	return filepath.Clean(abs)
}
