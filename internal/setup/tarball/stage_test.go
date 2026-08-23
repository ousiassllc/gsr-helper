package tarball

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// 一時展開領域は展開先の中に作る。成功したら残さず、前回の残骸も掃除する。
func TestExtractLeavesNoStagingDir(t *testing.T) {
	src := writeTarGz(t, []tarEntry{
		dirEntry("./bin", 0o755),
		regEntry("./bin/runsvc.sh", 0o755, "#!/bin/sh\n"),
		symEntry("./bin/alias.sh", "runsvc.sh"),
		regEntry("./config.sh", 0o750, "config\n"),
	})
	dest := t.TempDir()

	// 強制終了で置き去りになった一時展開領域を模す。
	stale := filepath.Join(dest, stagePrefix+"stale")
	if err := os.MkdirAll(filepath.Join(stale, "bin"), 0o755); err != nil {
		t.Fatalf("%s の作成に失敗した: %v", stale, err)
	}

	if err := Extract(t.Context(), src, dest, PreservedNames()); err != nil {
		t.Fatalf("Extract がエラーを返した: %v", err)
	}

	if got, want := dirNames(t, dest), []string{"bin", "config.sh"}; !slices.Equal(got, want) {
		t.Errorf("展開先の中身が %v（期待 %v）", got, want)
	}
	got, want := dirNames(t, filepath.Join(dest, "bin")), []string{"alias.sh", "runsvc.sh"}
	if !slices.Equal(got, want) {
		t.Errorf("bin の中身が %v（期待 %v）", got, want)
	}
}

// 展開に失敗したら一時展開領域ごと捨て、既存のファイルには手を付けない。
func TestExtractCleansStagingOnFailure(t *testing.T) {
	const body = "既存の config.sh"

	dest := t.TempDir()
	kept := filepath.Join(dest, "config.sh")
	if err := os.WriteFile(kept, []byte(body), 0o600); err != nil {
		t.Fatalf("%s の作成に失敗した: %v", kept, err)
	}

	// 1 件目は展開できるが 2 件目で弾かれる tar。途中まで書いた分を残さない。
	src := writeTarGz(t, []tarEntry{
		regEntry("./config.sh", 0o755, "新しい config.sh"),
		regEntry("../evil.txt", 0o644, "evil"),
	})

	if err := Extract(t.Context(), src, dest, PreservedNames()); !errors.Is(err, ErrUnsafePath) {
		t.Fatalf("エラーが ErrUnsafePath ではない: %v", err)
	}
	if got := readFile(t, kept); got != body {
		t.Errorf("既存の config.sh が %q に変わっている（期待 %q）", got, body)
	}
	if got, want := dirNames(t, dest), []string{"config.sh"}; !slices.Equal(got, want) {
		t.Errorf("展開先の中身が %v（期待 %v）", got, want)
	}
}

// dirNames はディレクトリ直下の名前を昇順で返す。
func dirNames(t *testing.T, dir string) []string {
	t.Helper()

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("%s の読み取りに失敗した: %v", dir, err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}
