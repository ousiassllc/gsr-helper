package confpath

// t.Setenv を使うため、このファイルのテストは t.Parallel() を付けない。

import (
	"errors"
	"io/fs"
	"os"
	"os/user"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// fakeUser はテスト用の os/user.User を作る。
func fakeUser(name, home string, uid, gid string) *user.User {
	return &user.User{Uid: uid, Gid: gid, Username: name, Name: name, HomeDir: home}
}

// fakeLookup は name が既知のときだけユーザーを返す lookup を作る。
// 呼ばれた引数を calls に記録し、文字種検証で弾かれたかを検証できるようにする。
func fakeLookup(known map[string]*user.User, calls *[]string) func(string) (*user.User, error) {
	return func(name string) (*user.User, error) {
		*calls = append(*calls, name)
		if u, ok := known[name]; ok {
			return u, nil
		}
		return nil, errors.New("unknown user")
	}
}

func TestResolveOwner(t *testing.T) {
	self := fakeUser("root", "/root", "0", "0")
	ousiass := fakeUser("ousiass", "/home/ousiass", "1000", "1000")
	known := map[string]*user.User{"ousiass": ousiass}

	tests := []struct {
		name      string
		sudoUser  string
		wantOwner Owner
		wantCall  bool
	}{
		{name: "未設定なら実行ユーザー", sudoUser: "", wantOwner: Owner{name: "root", home: "/root", uid: 0, gid: 0}},
		{
			name:      "設定されていれば SUDO_USER",
			sudoUser:  "ousiass",
			wantOwner: Owner{name: "ousiass", home: "/home/ousiass", uid: 1000, gid: 1000},
			wantCall:  true,
		},
		{
			name:      "存在しないユーザーなら実行ユーザー",
			sudoUser:  "nosuchuser",
			wantOwner: Owner{name: "root", home: "/root", uid: 0, gid: 0},
			wantCall:  true,
		},
		{name: "- 始まりは検証で弾く", sudoUser: "-x", wantOwner: Owner{name: "root", home: "/root", uid: 0, gid: 0}},
		{name: "空白入りは検証で弾く", sudoUser: "ou siass", wantOwner: Owner{name: "root", home: "/root", uid: 0, gid: 0}},
		{
			name:      "33 文字は検証で弾く",
			sudoUser:  strings.Repeat("a", maxUserNameLen+1),
			wantOwner: Owner{name: "root", home: "/root", uid: 0, gid: 0},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls []string
			got, err := resolveOwner(tt.sudoUser, fakeLookup(known, &calls), func() (*user.User, error) {
				return self, nil
			})
			if err != nil {
				t.Fatalf("resolveOwner() でエラー: %v", err)
			}
			if got != tt.wantOwner {
				t.Errorf("resolveOwner() = %+v, want %+v", got, tt.wantOwner)
			}
			if gotCall := len(calls) > 0; gotCall != tt.wantCall {
				t.Errorf("lookup の呼び出し = %v, want %v (calls=%v)", gotCall, tt.wantCall, calls)
			}
		})
	}
}

func TestResolveOwnerSelfError(t *testing.T) {
	var calls []string
	_, err := resolveOwner("", fakeLookup(nil, &calls), func() (*user.User, error) {
		return nil, errors.New("boom")
	})
	if err == nil {
		t.Fatal("実行ユーザーの取得に失敗してもエラーを返していない")
	}
}

func TestConfigPath(t *testing.T) {
	o := Owner{name: "ousiass", home: "/home/ousiass", uid: 1000, gid: 1000}

	tests := []struct {
		name string
		xdg  string
		want string
	}{
		{name: "未設定", xdg: "", want: "/home/ousiass/.config/gsr-helper/config.yaml"},
		{name: "ホーム配下の絶対パスは採用", xdg: "/home/ousiass/cfg", want: "/home/ousiass/cfg/gsr-helper/config.yaml"},
		{name: "相対パスは無視", xdg: "cfg", want: "/home/ousiass/.config/gsr-helper/config.yaml"},
		{name: "ホーム外は無視", xdg: "/root/.config", want: "/home/ousiass/.config/gsr-helper/config.yaml"},
		{name: "似た名前のホーム外は無視", xdg: "/home/ousiass2/.config", want: "/home/ousiass/.config/gsr-helper/config.yaml"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := configPath(o, tt.xdg)
			if got != tt.want {
				t.Errorf("configPath() = %s, want %s", got, tt.want)
			}
			if !strings.HasSuffix(got, filepath.Join(DirName, FileName)) {
				t.Errorf("configPath() が %s で終わっていない: %s", filepath.Join(DirName, FileName), got)
			}
		})
	}
}

// sudo 下で root のホームに書かないこと。XDG_CONFIG_HOME に root の値が
// 引き継がれていても、配置先は SUDO_USER のホームでなければならない。
//
// 実行ユーザーではなく lookup を差し替えて uid 1000 を与える。実効 UID に
// 依存しないので、root で make test しても sudo 下の本来の状況を検証できる。
func TestConfigPathAvoidsRootHome(t *testing.T) {
	t.Setenv(EnvXDGConfigHome, "/root/.config")
	known := map[string]*user.User{"ousiass": fakeUser("ousiass", "/home/ousiass", "1000", "1000")}

	var calls []string
	o, err := resolveOwner("ousiass", fakeLookup(known, &calls), func() (*user.User, error) {
		return fakeUser("root", "/root", "0", "0"), nil
	})
	if err != nil {
		t.Fatalf("resolveOwner() でエラー: %v", err)
	}
	got := o.ConfigPath()
	if want := "/home/ousiass/.config/gsr-helper/config.yaml"; got != want {
		t.Errorf("ConfigPath() = %s, want %s", got, want)
	}
}

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

func TestValidUserName(t *testing.T) {
	tests := map[string]bool{
		"ousiass":                             true,
		"a.b_c-d":                             true,
		"":                                    false,
		"-x":                                  false,
		"ou siass":                            false,
		"ousiass$":                            false,
		".":                                   false,
		"..":                                  false,
		strings.Repeat("a", maxUserNameLen):   true,
		strings.Repeat("a", maxUserNameLen+1): false,
	}
	for name, want := range tests {
		if got := validUserName(name); got != want {
			t.Errorf("validUserName(%q) = %v, want %v", name, got, want)
		}
	}
}
