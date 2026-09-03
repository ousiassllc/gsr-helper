package runner

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestFindRunnerDirs(t *testing.T) {
	base := t.TempDir()
	lvl1 := mkRunner(t, filepath.Join(base, "lvl1"))
	lvl2 := mkRunner(t, filepath.Join(base, "a", "lvl2"))
	lvl3 := mkRunner(t, filepath.Join(base, "a", "b", "lvl3"))
	mkRunner(t, filepath.Join(lvl1, "sub"))        // runner 配下は掘らない（FR-01）
	mkRunner(t, filepath.Join(base, "_work", "r")) // 走査対象外
	mkRunner(t, filepath.Join(base, "_diag", "r")) // 走査対象外
	file := filepath.Join(base, "file.txt")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name  string
		root  string
		depth int
		want  []string
	}{
		{"ルート自身が runner", lvl1, 0, []string{lvl1}},
		{"ルート自身が runner（深さ 2）", lvl1, 2, []string{lvl1}},
		{"深さ 0 では掘らない", base, 0, nil},
		{"深さ 1", base, 1, []string{lvl1}},
		{"深さ 2", base, 2, []string{lvl1, lvl2}},
		{"深さ 3", base, 3, []string{lvl1, lvl2, lvl3}},
		{"深さが負", base, -1, nil},
		{"存在しないルート", filepath.Join(base, "nope"), 2, nil},
		{"ファイルをルート指定", file, 2, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 検証したいのは「どのディレクトリを見つけるか」なので、
			// os.ReadDir の順序に依存しないよう両方を並べて比較する。
			got := findRunnerDirs(tt.root, tt.depth)
			slices.Sort(got)
			slices.Sort(tt.want)
			if !slices.Equal(got, tt.want) {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNormalizeDir(t *testing.T) {
	base := t.TempDir()
	target := mkDir(t, filepath.Join(base, "target"), nil)
	link := filepath.Join(base, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(base, "missing", "sub")

	tests := []struct{ name, in, want string }{
		{"空文字", "", ""},
		{"シンボリックリンクを解決する", link, target},
		{"末尾スラッシュを落とす", target + "/", target},
		{".. を畳む", filepath.Join(target, "..", "target"), target},
		{"存在しないパスでも絶対パスにする", missing, missing},
	}
	for _, tt := range tests {
		if got := normalizeDir(tt.in); got != tt.want {
			t.Errorf("%s: got %q, want %q", tt.name, got, tt.want)
		}
	}
	if got := normalizeDir("."); !filepath.IsAbs(got) {
		t.Errorf(`normalizeDir(".") = %q, want 絶対パス`, got)
	}
}

func TestCollectDirs(t *testing.T) {
	base := normalizeDir(t.TempDir())
	scanRoot := filepath.Join(base, "scan")
	shallow := mkRunner(t, filepath.Join(scanRoot, "r1"))
	deep := mkRunner(t, filepath.Join(scanRoot, "a", "r2"))
	fromProc := mkRunner(t, filepath.Join(base, "outside-proc"))
	fromUnit := mkRunner(t, filepath.Join(base, "outside-unit"))
	plain := mkDir(t, filepath.Join(base, "plain"), nil)
	link := filepath.Join(base, "link-to-r1")
	if err := os.Symlink(shallow, link); err != nil {
		t.Fatal(err)
	}

	proc := func(dirs ...string) []Process {
		out := make([]Process, 0, len(dirs))
		for i, d := range dirs {
			out = append(out, Process{PID: i + 1, Dir: d})
		}
		return out
	}
	unit := func(dir string) []SvcState { return []SvcState{{Unit: u1, WorkingDir: dir}} }
	// SkipDefaultRoots で既定ルート（実ホストの設置場所）を外し、結果を
	// 完全一致で比較できるようにする。
	scan := Options{Roots: []string{scanRoot}, SkipDefaultRoots: true}
	scan1 := Options{Roots: []string{scanRoot}, Depth: 1, SkipDefaultRoots: true}
	none := Options{SkipDefaultRoots: true}

	tests := []struct {
		name  string
		opts  Options
		procs []Process
		units []SvcState
		want  []string
	}{
		{"走査ルートのみ（Depth 0 は既定の 2）", scan, nil, nil, []string{deep, shallow}},
		{"深さ 1 では深い runner を拾わない", scan1, nil, nil, []string{shallow}},
		{"プロセス由来の走査ルート外 runner を回収（FR-02）", none, proc(fromProc), nil, []string{fromProc}},
		{"ユニット由来の走査ルート外 runner を回収（FR-02）", none, nil, unit(fromUnit), []string{fromUnit}},
		{"3 経路で同一ディレクトリなら 1 件", scan1, proc(shallow), unit(shallow), []string{shallow}},
		{"シンボリックリンク経由と実パスは同一", scan1, proc(link), nil, []string{shallow}},
		{".runner を持たないディレクトリ・空の Dir は除外", none, proc(plain, ""), unit(filepath.Join(base, "gone")), nil},
		{"結果はソート済み", none, proc(fromUnit, fromProc), nil, []string{fromProc, fromUnit}},
		// 既定ルートを外しても FR-02 の補完は止まらない一方、走査ルートは 0 件になる。
		{"既定ルートを使わない指定でルートも空なら走査由来は 0 件", none, nil, nil, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := collectDirs(tt.opts, tt.procs, tt.units)
			if !slices.Equal(got, tt.want) {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}
