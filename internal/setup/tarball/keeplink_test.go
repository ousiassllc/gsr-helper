package tarball

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// FR-21: 保持対象へリンクで潜り込むエントリを拒否すること。
//
// os.Root は展開先の外への書き込みを止めるが、展開先の中にある .runner を
// リンク越しに書き換えるのは止めない。保持を破られるとバージョン更新のたびに
// runner の登録がやり直しになるため、保持側で明示的に弾く必要がある。
func TestExtractRejectsLinksIntoPreservedNames(t *testing.T) {
	tests := []struct {
		name string
		// setup は展開前に置いておく保持対象を作り、あとで中身を確かめるパスと
		// 期待する中身を返す。
		setup func(t *testing.T, dest string) (string, string)
		// entries は展開させる tar の中身。
		entries []tarEntry
		// absent は展開後に存在してはならないパス（dest からの相対）。
		absent string
	}{
		{
			// シンボリックリンクで .runner を指し、その配下へ書き込ませる。
			name:  "シンボリックリンク経由で保持対象の中へ書き込む",
			setup: preservedDir,
			entries: []tarEntry{
				symEntry("./link", ".runner"),
				regEntry("./link/pwned", 0o644, "pwned"),
			},
			absent: ".runner/pwned",
		},
		{
			// ハードリンクで .runner と同じ inode に別名を付け、その名前を
			// 通常ファイルとして開き直して中身を潰す。
			name:  "ハードリンク経由で保持対象を書き潰す",
			setup: preservedFile,
			entries: []tarEntry{
				linkEntry("./link", ".runner"),
				regEntry("./link", 0o644, "pwned"),
			},
			absent: "link",
		},
		{
			// 途中の階層もリンクにして、保持対象までを多段で辿らせる。名前を
			// 1 要素ずつ辿らずに引き当てるだけだと、d/link の向き先を d/.runner と
			// 誤読して素通りし、d/link への書き込みが .runner を潰す。
			name:  "多段のリンクを辿った先が保持対象",
			setup: preservedFile,
			entries: []tarEntry{
				symEntry("./d", "."),
				symEntry("./d/link", ".runner"),
				regEntry("./d/link", 0o644, "pwned"),
			},
			absent: "link",
		},
		{
			// リンク自身の名前が保持対象の場合も、置き換えさせない。
			name:    "保持対象そのものを指すシンボリックリンク",
			setup:   preservedFile,
			entries: []tarEntry{symEntry("./.runner", "bin/config.sh")},
			absent:  "bin",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dest := t.TempDir()
			path, want := tt.setup(t, dest)

			err := Extract(t.Context(), writeTarGz(t, tt.entries), dest, PreservedNames())
			if err == nil {
				t.Fatal("Extract が保持対象へ潜り込むリンクを受け入れた")
			}
			if !errors.Is(err, ErrPreservedLink) {
				t.Errorf("エラーが ErrPreservedLink ではない: %v", err)
			}

			if got := readFile(t, path); got != want {
				t.Errorf("保持対象が壊れている: %q（期待 %q）", got, want)
			}
			absent := filepath.Join(dest, filepath.FromSlash(tt.absent))
			if _, serr := os.Lstat(absent); !errors.Is(serr, os.ErrNotExist) {
				t.Errorf("%s が作られている: %v", tt.absent, serr)
			}
		})
	}
}

// preservedDir は登録情報をディレクトリとして置き、中の目印のパスと中身を返す。
func preservedDir(t *testing.T, dest string) (string, string) {
	t.Helper()

	const body = "既存の登録情報"

	dir := filepath.Join(dest, ".runner")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("%s の作成に失敗した: %v", dir, err)
	}
	path := filepath.Join(dir, "marker")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("%s の作成に失敗した: %v", path, err)
	}
	return path, body
}

// preservedFile は登録情報をファイルとして置き、そのパスと中身を返す。
func preservedFile(t *testing.T, dest string) (string, string) {
	t.Helper()

	const body = `{"agentId":1}`

	path := filepath.Join(dest, ".runner")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("%s の作成に失敗した: %v", path, err)
	}
	return path, body
}
