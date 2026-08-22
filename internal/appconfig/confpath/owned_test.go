package confpath

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// openHome はテスト用に home を根とする os.Root を開く。
func openHome(t *testing.T, home string) *os.Root {
	t.Helper()

	root, err := os.OpenRoot(home)
	if err != nil {
		t.Fatalf("準備に失敗: %v", err)
	}
	t.Cleanup(func() { _ = root.Close() })
	return root
}

// recorder は副作用を記録するだけの fsOps を作る。記録は絶対パスに直して返すので、
// 「実際にどのパスを触ったか」がそのまま読める。
func recorder(home string, chmodded, chowned *[]string) fsOps {
	return fsOps{
		geteuid: func() int { return 0 },
		chmod: func(_ *os.Root, p string, _ os.FileMode) error {
			*chmodded = append(*chmodded, filepath.Join(home, p))
			return nil
		},
		lchown: func(_ *os.Root, p string, _, _ int) error {
			*chowned = append(*chowned, filepath.Join(home, p))
			return nil
		},
	}
}

// chown はリンクを辿るため、対象ユーザーが差し替えたリンク先を root が chown
// させられる。lchown はリンク自体を対象にするので、行き先の無いリンクでも成功する
// （os.Chown だとこの呼び出しが ENOENT で落ちる）。
func TestChownDirsDoesNotFollowSymlink(t *testing.T) {
	home := t.TempDir()
	target := filepath.Join(home, "target") // 作らない = リンク先が無い状態
	if err := os.Symlink(target, filepath.Join(home, DirName)); err != nil {
		t.Fatalf("準備に失敗: %v", err)
	}

	root := openHome(t, home)
	if err := chownDirs(root, []string{DirName}, os.Geteuid(), os.Getegid(), realFS.lchown); err != nil {
		t.Fatalf("chownDirs() でエラー（リンクを辿っている）: %v", err)
	}
	if _, err := os.Lstat(target); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("リンク先に触っている: %v", err)
	}
}

// root 実行時に締め直す対象は「作ったディレクトリ」と leaf だけであること。
func TestMkdirOwnedFSTargets(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, ".config", DirName)

	var chowned, chmodded []string
	o := Owner{name: "u", home: home, uid: 1000, gid: 1000}
	if err := o.mkdirOwnedFS(dir, recorder(home, &chmodded, &chowned)); err != nil {
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

// 列挙するのは「まだ存在しない要素」だけで、根（ホーム）自身は含めないこと。
func TestMissingDirs(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, "c"), DirMode); err != nil {
		t.Fatalf("準備に失敗: %v", err)
	}
	root := openHome(t, home)

	got := missingDirs(root, filepath.Join("a", "b"))
	want := []string{"a", filepath.Join("a", "b")}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("missingDirs() = %v, want %v", got, want)
	}
	if got := missingDirs(root, "."); got != nil {
		t.Errorf("ホーム自身が対象になっている: %v", got)
	}
	if got, want := missingDirs(root, filepath.Join("c", "d")), []string{filepath.Join("c", "d")}; !reflect.DeepEqual(got, want) {
		t.Errorf("既存の祖先で打ち切っていない: %v, want %v", got, want)
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
	o := Owner{name: "u", home: home, uid: 1000, gid: 1000}
	if err := o.mkdirOwnedFS(dir, recorder(home, &chmodded, &chowned)); err != nil {
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

// leaf がリンクなら mode を締め直さないこと。
//
// os.Root はホームの外へ出る参照を拒むが、ホーム内で完結する相対リンクは辿る。
// leaf をそうしたリンクに差し替えられると、root がリンク先のファイルを 0700 に
// してしまう（世界から読めていたファイルを読めなくする）。
func TestChmodLeafRejectsSymlink(t *testing.T) {
	home := t.TempDir()
	target := filepath.Join(home, "target")
	if err := os.WriteFile(target, []byte("x"), 0o644); err != nil {
		t.Fatalf("準備に失敗: %v", err)
	}
	// ホーム内で完結する相対リンク。os.Root だけでは辿れてしまう形。
	if err := os.Symlink("target", filepath.Join(home, DirName)); err != nil {
		t.Fatalf("準備に失敗: %v", err)
	}

	root := openHome(t, home)
	if err := chmodLeaf(root, DirName, realFS.chmod); err == nil {
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

// 経路の途中の要素がホーム外へのシンボリックリンクだった場合、mode も所有者も
// 変えず、リンクの先にディレクトリも作らないこと。
//
// underHome はパス名だけの字句判定で、O_NOFOLLOW は最後の要素しか守らない。
// <home>/.config を /etc へのリンクに差し替えられると、字句上はホーム配下の
// パスのまま root が /etc/gsr-helper を 0700 にし、非特権ユーザー所有へ chown
// してしまう。本ツールへの sudo だけを許されたユーザーにとっては権限昇格になる。
func TestMkdirOwnedFSRejectsSymlinkedAncestor(t *testing.T) {
	base := t.TempDir()
	home := filepath.Join(base, "home")
	outside := filepath.Join(base, "etc")
	if err := os.MkdirAll(home, DirMode); err != nil {
		t.Fatalf("準備に失敗: %v", err)
	}
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatalf("準備に失敗: %v", err)
	}
	// 対象ユーザーが自分のホームの中で <home>/.config をホーム外へのリンクに差し替える。
	if err := os.Symlink(outside, filepath.Join(home, ".config")); err != nil {
		t.Fatalf("準備に失敗: %v", err)
	}
	dir := filepath.Join(home, ".config", DirName)

	var chowned, chmodded []string
	o := Owner{name: "u", home: home, uid: 1000, gid: 1000}
	if err := o.mkdirOwnedFS(dir, recorder(home, &chmodded, &chowned)); err == nil {
		t.Error("ホームの外へ出る経路を拒否していない")
	}
	if len(chmodded) != 0 {
		t.Errorf("ホーム外のパーミッションを変更している: %v", chmodded)
	}
	if len(chowned) != 0 {
		t.Errorf("ホーム外の所有者を変更している: %v", chowned)
	}
	if _, err := os.Lstat(filepath.Join(outside, DirName)); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("ホームの外にディレクトリを作っている: %v", err)
	}
}
