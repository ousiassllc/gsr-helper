package confpath

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// chown はリンクを辿るため、対象ユーザーが差し替えたリンク先を root が chown
// させられる。lchown はリンク自体を対象にするので、行き先の無いリンクでも成功する
// （os.Chown だとこの呼び出しが ENOENT で落ちる）。
func TestChownDirsDoesNotFollowSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target") // 作らない = リンク先が無い状態
	link := filepath.Join(dir, DirName)
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("準備に失敗: %v", err)
	}
	if err := chownDirs([]string{link}, os.Geteuid(), os.Getegid(), realFS.lchown); err != nil {
		t.Fatalf("chownDirs() でエラー（リンクを辿っている）: %v", err)
	}
	if _, err := os.Stat(target); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("リンク先に触っている: %v", err)
	}
}

// root 実行時に締め直す対象は「作ったディレクトリ」と leaf だけであること。
func TestMkdirOwnedFSTargets(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, ".config", DirName)

	var chowned, chmodded []string
	ops := fsOps{
		geteuid: func() int { return 0 },
		chmod:   func(p string, _ os.FileMode) error { chmodded = append(chmodded, p); return nil },
		lchown:  func(p string, _, _ int) error { chowned = append(chowned, p); return nil },
	}
	o := Owner{name: "u", home: home, uid: 1000, gid: 1000}
	if err := o.mkdirOwnedFS(dir, ops); err != nil {
		t.Fatalf("mkdirOwnedFS() でエラー: %v", err)
	}
	if want := []string{filepath.Join(home, ".config"), dir}; !reflect.DeepEqual(chowned, want) {
		t.Errorf("所有者変更の対象 = %v, want %v", chowned, want)
	}
	if want := []string{dir}; !reflect.DeepEqual(chmodded, want) {
		t.Errorf("パーミッション変更の対象 = %v, want %v", chmodded, want)
	}
}

// 対象ユーザーのホームが無いときは何も作らずエラーにすること。root が作ると
// root 所有 0700 のホームができ、当該ユーザーが自分の設定を読めなくなる。
func TestMkdirOwnedRequiresExistingHome(t *testing.T) {
	home := filepath.Join(t.TempDir(), "newuser")
	dir := filepath.Join(home, ".config", DirName)

	o := Owner{name: "newuser", home: home, uid: 1000, gid: 1000}
	if err := o.MkdirOwned(dir); err == nil {
		t.Fatal("ホームが無いのにエラーを返していない")
	}
	if _, err := os.Stat(home); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("ホームを作っている: %v", err)
	}
}

func TestChownTarget(t *testing.T) {
	tests := []struct {
		name             string
		euid             int
		own              Owner
		wantUID, wantGID int
		wantOK           bool
	}{
		{name: "非 root では chown しない", euid: 1000, own: Owner{uid: 1000, gid: 1000}},
		{name: "root 実行で root 所有なら不要", euid: 0, own: Owner{uid: 0, gid: 0}},
		{name: "root 実行で別ユーザー所有なら chown", euid: 0, own: Owner{uid: 1000, gid: 1000}, wantUID: 1000, wantGID: 1000, wantOK: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uid, gid, ok := tt.own.ChownTarget(tt.euid)
			if uid != tt.wantUID || gid != tt.wantGID || ok != tt.wantOK {
				t.Errorf("ChownTarget() = (%d, %d, %v), want (%d, %d, %v)", uid, gid, ok, tt.wantUID, tt.wantGID, tt.wantOK)
			}
		})
	}
}

// ホームより上のディレクトリは chown の対象にしないこと。
func TestMissingDirs(t *testing.T) {
	home := t.TempDir()

	got := missingDirs(filepath.Join(home, "a", "b"), home)
	want := []string{filepath.Join(home, "a"), filepath.Join(home, "a", "b")}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("missingDirs() = %v, want %v", got, want)
	}
	if got := missingDirs(home, home); got != nil {
		t.Errorf("ホーム自身が対象になっている: %v", got)
	}
	if got := missingDirs("/nonexistent/x/y", home); got != nil {
		t.Errorf("ホーム外が対象になっている: %v", got)
	}
	if got := missingDirs("/home/x/y", "/"); got != nil {
		t.Errorf("ホームが / のとき対象が空でない: %v", got)
	}
}

// ホームの外にあるディレクトリには mode も所有者も触らないこと。
//
// 名前だけで leaf と判断すると、--config /etc/gsr-helper/config.yaml のような指定で
// root が /etc/gsr-helper を 0700 にし、さらに非特権ユーザー所有へ chown してしまう。
// 本ツールへの sudo だけを許されたユーザーにとっては権限昇格になる。
func TestMkdirOwnedFSLeavesPathsOutsideHomeAlone(t *testing.T) {
	home := t.TempDir()
	// ホームとは無関係な場所にある、名前だけが leaf と同じディレクトリ。
	dir := filepath.Join(t.TempDir(), "etc", DirName)

	var chowned, chmodded []string
	ops := fsOps{
		geteuid: func() int { return 0 },
		chmod:   func(p string, _ os.FileMode) error { chmodded = append(chmodded, p); return nil },
		lchown:  func(p string, _, _ int) error { chowned = append(chowned, p); return nil },
	}
	o := Owner{name: "u", home: home, uid: 1000, gid: 1000}
	if err := o.mkdirOwnedFS(dir, ops); err != nil {
		t.Fatalf("mkdirOwnedFS() でエラー: %v", err)
	}
	if len(chmodded) != 0 {
		t.Errorf("ホーム外のパーミッションを変更している: %v", chmodded)
	}
	if len(chowned) != 0 {
		t.Errorf("ホーム外の所有者を変更している: %v", chowned)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Errorf("ディレクトリ自体は作られるべき: %v", err)
	}
}

// chmod もリンクを辿らないこと。
//
// leaf をシンボリックリンクに差し替えられると、root がリンク先のファイルを 0700 に
// してしまう（世界から読めていたファイルを読めなくする）。lchown と同じ脅威なので
// 同じ方針で断つ。
func TestRealFSChmodDoesNotFollowSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	if err := os.WriteFile(target, []byte("x"), 0o644); err != nil {
		t.Fatalf("準備に失敗: %v", err)
	}
	link := filepath.Join(dir, DirName)
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("準備に失敗: %v", err)
	}

	if err := realFS.chmod(link, DirMode); err == nil {
		t.Error("シンボリックリンクへの chmod が成功している")
	}
	fi, err := os.Stat(target)
	if err != nil {
		t.Fatalf("Stat に失敗: %v", err)
	}
	if got := fi.Mode().Perm(); got != 0o644 {
		t.Errorf("リンク先の mode が変わっている: %o, want 644", got)
	}
}
