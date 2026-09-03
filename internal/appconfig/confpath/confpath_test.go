package confpath

// t.Setenv を使うため、このファイルのテストは t.Parallel() を付けない。

import (
	"errors"
	"os/user"
	"path/filepath"
	"strings"
	"testing"
)

// fakeUser はテスト用の os/user.User を作る。
func fakeUser(name, home string, uid, gid string) *user.User {
	return &user.User{Uid: uid, Gid: gid, Username: name, Name: name, HomeDir: home}
}

// alwaysDir はホームの存在確認を常に真にする。ホームの有無そのものを見るテスト
// （TestResolveOwnerFallsBackWhenHomeMissing）以外は関心が無いためである。
func alwaysDir(string) bool { return true }

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
			}, alwaysDir)
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
	}, alwaysDir)
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
	}, alwaysDir)
	if err != nil {
		t.Fatalf("resolveOwner() でエラー: %v", err)
	}
	got := o.ConfigPath()
	if want := "/home/ousiass/.config/gsr-helper/config.yaml"; got != want {
		t.Errorf("ConfigPath() = %s, want %s", got, want)
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

// ホームを持たないユーザー（サービスアカウントなど）で sudo された場合は、
// 設定が保存できなくなるより実行ユーザーの設定ディレクトリに倒すこと。
func TestResolveOwnerFallsBackWhenHomeMissing(t *testing.T) {
	known := map[string]*user.User{"svc": fakeUser("svc", "/nonexistent", "1001", "1001")}
	var calls []string

	got, err := resolveOwner("svc", fakeLookup(known, &calls), func() (*user.User, error) {
		return fakeUser("root", "/root", "0", "0"), nil
	}, func(string) bool { return false })
	if err != nil {
		t.Fatalf("resolveOwner() でエラー: %v", err)
	}
	if want := (Owner{name: "root", home: "/root", uid: 0, gid: 0}); got != want {
		t.Errorf("resolveOwner() = %+v, want %+v", got, want)
	}
}
