package tarball

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestExtractWritesFilesDirsAndLinks(t *testing.T) {
	src := writeTarGz(t, []tarEntry{
		dirEntry("./", 0o755),
		dirEntry("./bin", 0o755),
		regEntry("./bin/runsvc.sh", 0o755, "#!/bin/sh\n"),
		regEntry("./config.sh", 0o750, "config\n"),
		regEntry("./bin/Runner.Listener", 0o600, "listener"),
		symEntry("./bin/alias.sh", "runsvc.sh"),
		linkEntry("./config-link.sh", "./config.sh"),
		fifoEntry("./pipe"),
	})
	dest := t.TempDir()

	if err := Extract(src, dest, PreservedNames()); err != nil {
		t.Fatalf("Extract がエラーを返した: %v", err)
	}

	if got := readFile(t, filepath.Join(dest, "bin", "runsvc.sh")); got != "#!/bin/sh\n" {
		t.Errorf("bin/runsvc.sh の中身が %q（期待 %q）", got, "#!/bin/sh\n")
	}
	if got := readFile(t, filepath.Join(dest, "config.sh")); got != "config\n" {
		t.Errorf("config.sh の中身が %q（期待 %q）", got, "config\n")
	}

	perms := map[string]os.FileMode{
		"bin":                 0o755,
		"bin/runsvc.sh":       0o755,
		"config.sh":           0o750,
		"bin/Runner.Listener": 0o600,
	}
	for name, want := range perms {
		if got := mustPerm(t, filepath.Join(dest, name)); got != want {
			t.Errorf("%s のパーミッションが %o（期待 %o）", name, got, want)
		}
	}

	link, err := os.Readlink(filepath.Join(dest, "bin", "alias.sh"))
	if err != nil {
		t.Fatalf("bin/alias.sh の Readlink に失敗した: %v", err)
	}
	if link != "runsvc.sh" {
		t.Errorf("bin/alias.sh のリンク先が %q（期待 %q）", link, "runsvc.sh")
	}

	if got := readFile(t, filepath.Join(dest, "config-link.sh")); got != "config\n" {
		t.Errorf("ハードリンク config-link.sh の中身が %q（期待 %q）", got, "config\n")
	}

	// FIFO のような種別は作らずに読み飛ばす。root 権限で作らせる理由が無い。
	if _, err := os.Lstat(filepath.Join(dest, "pipe")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("対象外の種別 pipe が作られている: %v", err)
	}
}

// FR-21: バージョン更新で runner 自身の状態を潰さないこと。
func TestExtractKeepsPreservedNames(t *testing.T) {
	src := writeTarGz(t, []tarEntry{
		regEntry("./.env", 0o644, "tarball 側の .env"),
		regEntry("./.runner", 0o644, "tarball 側の .runner"),
		dirEntry("./_work", 0o755),
		regEntry("./_work/keep.txt", 0o644, "tarball 側の作業ファイル"),
		regEntry("./bin/installdependencies.sh", 0o755, "新しい本体"),
	})
	dest := t.TempDir()

	existing := map[string]string{
		".env":           "既存の .env",
		".runner":        "既存の .runner",
		"_work/keep.txt": "既存の作業ファイル",
	}
	for name, body := range existing {
		path := filepath.Join(dest, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("%s の親ディレクトリ作成に失敗した: %v", name, err)
		}
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatalf("%s の作成に失敗した: %v", name, err)
		}
	}

	if err := Extract(src, dest, PreservedNames()); err != nil {
		t.Fatalf("Extract がエラーを返した: %v", err)
	}

	for name, want := range existing {
		if got := readFile(t, filepath.Join(dest, filepath.FromSlash(name))); got != want {
			t.Errorf("%s が上書きされている: %q（期待 %q）", name, got, want)
		}
	}
	// 保持対象以外は通常どおり展開される。
	if got := readFile(t, filepath.Join(dest, "bin", "installdependencies.sh")); got != "新しい本体" {
		t.Errorf("bin/installdependencies.sh が展開されていない: %q", got)
	}
}

// zip-slip 対策。展開先の外へ 1 バイトも書かせないことを実物で確かめる。
func TestExtractRejectsPathsOutsideDest(t *testing.T) {
	tests := []struct {
		name string
		// entries は展開させる tar の中身。
		entries []tarEntry
	}{
		{
			name:    ".. を含む相対パス",
			entries: []tarEntry{regEntry("../evil.txt", 0o644, "evil")},
		},
		{
			name:    "深い階層からの .. 抜け",
			entries: []tarEntry{regEntry("./bin/../../evil.txt", 0o644, "evil")},
		},
		{
			name:    "絶対パス",
			entries: []tarEntry{regEntry("/etc/evil.txt", 0o644, "evil")},
		},
		{
			name:    "展開先の外を指すシンボリックリンク",
			entries: []tarEntry{symEntry("./escape", "../../outside")},
		},
		{
			name:    "絶対パスを指すシンボリックリンク",
			entries: []tarEntry{symEntry("./escape", "/etc")},
		},
		{
			name:    "展開先の外を指すハードリンク",
			entries: []tarEntry{linkEntry("./escape", "../../etc/passwd")},
		},
		{
			name: "リンク経由で外へ書き込むエントリ",
			entries: []tarEntry{
				symEntry("./escape", "../outside"),
				regEntry("./escape/evil.txt", 0o644, "evil"),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			dest := filepath.Join(root, "runner")
			outside := filepath.Join(root, "outside")
			if err := os.MkdirAll(outside, 0o755); err != nil {
				t.Fatalf("%s の作成に失敗した: %v", outside, err)
			}

			err := Extract(writeTarGz(t, tt.entries), dest, PreservedNames())
			if err == nil {
				t.Fatal("Extract が展開先の外を指すエントリを受け入れた")
			}
			if !errors.Is(err, ErrUnsafePath) {
				t.Errorf("エラーが ErrUnsafePath ではない: %v", err)
			}

			entries, readErr := os.ReadDir(outside)
			if readErr != nil {
				t.Fatalf("%s の読み取りに失敗した: %v", outside, readErr)
			}
			if len(entries) != 0 {
				t.Errorf("展開先の外に %d 件書き出されている", len(entries))
			}
			if _, statErr := os.Lstat(filepath.Join(root, "evil.txt")); !errors.Is(statErr, os.ErrNotExist) {
				t.Errorf("展開先の外に evil.txt が作られている: %v", statErr)
			}
		})
	}
}

// decompression bomb 対策。上限は定数だが、テストからは小さい値を渡して確かめる。
func TestExtractRejectsOversizedEntries(t *testing.T) {
	tests := []struct {
		name    string
		entries []tarEntry
		lim     limits
	}{
		{
			name:    "1 エントリが上限を超える",
			entries: []tarEntry{regEntry("./big.bin", 0o644, "0123456789")},
			lim:     limits{total: 100, entry: 5},
		},
		{
			name: "合計が上限を超える",
			entries: []tarEntry{
				regEntry("./a.bin", 0o644, "0123456789"),
				regEntry("./b.bin", 0o644, "0123456789"),
			},
			lim: limits{total: 15, entry: 10},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dest := t.TempDir()

			err := extractTo(writeTarGz(t, tt.entries), dest, PreservedNames(), tt.lim)
			if err == nil {
				t.Fatal("extractTo が上限を超える展開を受け入れた")
			}
			if !errors.Is(err, ErrTooLarge) {
				t.Errorf("エラーが ErrTooLarge ではない: %v", err)
			}
		})
	}
}

func TestExtractDefaultLimits(t *testing.T) {
	if MaxTotalBytes != 2<<30 {
		t.Errorf("MaxTotalBytes が %d（期待 %d）", MaxTotalBytes, int64(2<<30))
	}
	if MaxEntryBytes > MaxTotalBytes {
		t.Errorf("MaxEntryBytes %d が MaxTotalBytes %d を超えている", MaxEntryBytes, MaxTotalBytes)
	}
}

func TestExtractRejectsBadInput(t *testing.T) {
	dest := t.TempDir()

	if err := Extract(filepath.Join(t.TempDir(), "no-such.tar.gz"), dest, nil); err == nil {
		t.Error("存在しない tarball でエラーにならなかった")
	}

	plain := filepath.Join(t.TempDir(), "plain.tar.gz")
	if err := os.WriteFile(plain, []byte("これは gzip ではない"), 0o600); err != nil {
		t.Fatalf("%s の作成に失敗した: %v", plain, err)
	}
	if err := Extract(plain, dest, nil); err == nil {
		t.Error("gzip でないファイルでエラーにならなかった")
	}

	if err := Extract(writeTarGz(t, nil), "", nil); err == nil {
		t.Error("展開先が空でエラーにならなかった")
	}
}
