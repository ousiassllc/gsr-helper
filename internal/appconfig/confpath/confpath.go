package appconfig

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

const (
	// envSudoUser / envXDGConfigHome は配置先の決定に読む環境変数。
	envSudoUser      = "SUDO_USER"
	envXDGConfigHome = "XDG_CONFIG_HOME"

	// 設定ファイルの相対位置。生成するパスは常に gsr-helper/config.yaml で終わる。
	appDirName     = "gsr-helper"
	configFileName = "config.yaml"

	// security.md の表のとおり、設定ファイルは 600、ディレクトリは 700。
	dirPerm  os.FileMode = 0o700
	filePerm os.FileMode = 0o600

	// maxUserNameLen は SUDO_USER として受け付ける長さの上限。
	maxUserNameLen = 32
)

// owner は設定ファイルを所有すべきユーザー。
type owner struct {
	name string
	home string
	uid  int
	gid  int
}

// fsOps は所有者・パーミッションを変える副作用。テストで差し替えるため関数値で持つ。
type fsOps struct {
	geteuid func() int
	chmod   func(string, os.FileMode) error
	lchown  func(string, int, int) error
}

// realFS は実環境の副作用。
//
// chown ではなく lchown を使う。chown(2) はリンクを辿るため、missingDirs の Stat →
// MkdirAll → 所有者変更の隙に対象ユーザーが <home>/.config/gsr-helper を任意パスへの
// シンボリックリンクへ差し替えると、root がそのリンク先を当該ユーザー所有に変えて
// しまう。lchown はリンク自体を対象にするので無害化される（internal/audit が
// O_NOFOLLOW で断っているのと同じ脅威に、同じ方針で対処する）。
var realFS = fsOps{geteuid: os.Geteuid, chmod: os.Chmod, lchown: os.Lchown}

// DefaultPath は設定ファイルの既定の配置先を返す。
//
// os.UserConfigDir は使わない。HOME を読むため sudo 下では /root を指しうるうえ、
// sudoers の env_keep / always_set_home 次第で挙動が変わる。
// os/user.Lookup(SUDO_USER).HomeDir から決めればホスト設定に依らず決定的になる。
func DefaultPath() (string, error) {
	o, err := defaultOwner()
	if err != nil {
		return "", err
	}
	return defaultPathFor(o), nil
}

// defaultPathFor は o にとっての既定の配置先を返す。
// XDG_CONFIG_HOME を読むのはここだけにして、環境変数の扱いを 1 か所に寄せる。
func defaultPathFor(o owner) string {
	return configPath(o, os.Getenv(envXDGConfigHome))
}

// defaultOwner は実環境から所有者を解決する。
func defaultOwner() (owner, error) {
	return resolveOwner(sudoUserFromEnv(os.Getenv), user.Lookup, user.Current)
}

// sudoUserFromEnv は SUDO_USER を読む。文字種が不正なら空を返す。
//
// 読み取りと検証をこの 1 か所に寄せる。同じ値を別々の場所で読むと検証の有無が
// 食い違い、「配置先は実行ユーザーなのに記録は生値」のようなずれが起きる。
func sudoUserFromEnv(getenv func(string) string) string {
	name := getenv(envSudoUser)
	if !validUserName(name) {
		return ""
	}
	return name
}

// resolveOwner は設定ファイルの所有者を決める。
//
// sudoUser が使えないとき（未設定・不正な文字種・存在しないユーザー）は実行ユーザーに
// フォールバックする。lookup / self を引数に取るのはテストで差し替えるためである。
func resolveOwner(
	sudoUser string,
	lookup func(string) (*user.User, error),
	self func() (*user.User, error),
) (owner, error) {
	if validUserName(sudoUser) {
		if u, err := lookup(sudoUser); err == nil {
			if o, oerr := toOwner(u); oerr == nil {
				return o, nil
			}
		}
	}
	u, err := self()
	if err != nil {
		return owner{}, fmt.Errorf("実行ユーザーの取得に失敗しました: %w", err)
	}
	return toOwner(u)
}

// toOwner は os/user の値を owner に変換する。
func toOwner(u *user.User) (owner, error) {
	uid, err := strconv.Atoi(u.Uid)
	if err != nil {
		return owner{}, fmt.Errorf("ユーザー %s の uid の解釈に失敗しました: %w", u.Username, err)
	}
	gid, err := strconv.Atoi(u.Gid)
	if err != nil {
		return owner{}, fmt.Errorf("ユーザー %s の gid の解釈に失敗しました: %w", u.Username, err)
	}
	return owner{name: u.Username, home: filepath.Clean(u.HomeDir), uid: uid, gid: gid}, nil
}

// configPath は所有者と XDG_CONFIG_HOME から設定ファイルのパスを組む純粋関数。
//
// XDG_CONFIG_HOME は「絶対パスかつ対象ユーザーのホーム配下」のときだけ尊重する。
// sudo で root の値が引き継がれた場合に root のホームへ書かないためである。
func configPath(o owner, xdgConfigHome string) string {
	base := filepath.Join(o.home, ".config")
	if x := filepath.Clean(xdgConfigHome); filepath.IsAbs(x) && underHome(o.home, x) {
		base = x
	}
	return filepath.Join(base, appDirName, configFileName)
}

// underHome は p が home と同じかその配下にあるかを返す。
// home が絶対パスでない場合と / の場合は「配下ではない」とする（/ を認めると
// あらゆるパスが配下になり、root のホームを弾く判定として機能しなくなる）。
func underHome(home, p string) bool {
	h := filepath.Clean(home)
	if !filepath.IsAbs(h) || h == string(filepath.Separator) {
		return false
	}
	c := filepath.Clean(p)
	return c == h || strings.HasPrefix(c, h+string(filepath.Separator))
}

// mkdirOwned は dir を 700 で作り、本ツールが責任を持つディレクトリだけを所有者に合わせる。
//
// MkdirAll の前に「存在しない祖先」を列挙し、その中でも対象ユーザーのホームより
// 下にあるものだけを chown 対象にする。こうしておくと、既存のディレクトリや
// /home のような共有ディレクトリを chown する事故が構造的に起こらない。
func mkdirOwned(dir string, o owner) error {
	return mkdirOwnedFS(dir, o, realFS)
}

// mkdirOwnedFS は副作用を差し替えられる本体。
func mkdirOwnedFS(dir string, o owner, ops fsOps) error {
	// ホームの作成は本ツールの責務ではない。root で作ると root 所有 0700 になり、
	// 対象ユーザーが自分のホームを通れず設定を読めなくなる（security.md 4.）。
	if fi, err := os.Stat(o.home); err != nil || !fi.IsDir() {
		return fmt.Errorf("%s のホームディレクトリ %s がありません", o.name, o.home)
	}

	targets := missingDirs(dir, o.home)
	if err := os.MkdirAll(dir, dirPerm); err != nil {
		return fmt.Errorf("%s の作成に失敗しました: %w", dir, err)
	}

	// leaf は本ツール専用のディレクトリなので、既存でも mode と所有者を締め直す。
	// root が 0755 で作ったまま残っていると、次に非 root で起動したときに一時
	// ファイルを作れず自分の設定を書けない（security.md 4.）。
	// ~/.config のような共有の祖先と、--config で指された任意のディレクトリは
	// 他の用途と共有されうるので触らない。線引きはディレクトリ名で行う。
	if filepath.Base(dir) == appDirName {
		if err := ops.chmod(dir, dirPerm); err != nil {
			return fmt.Errorf("%s のパーミッション設定に失敗しました: %w", dir, err)
		}
		if !slices.Contains(targets, dir) {
			targets = append(targets, dir)
		}
	}

	uid, gid, ok := chownTarget(ops.geteuid(), o)
	if !ok {
		return nil
	}
	return chownDirs(targets, uid, gid, ops.lchown)
}

// chownDirs は dirs の所有者を uid/gid に合わせる。
// リンクを辿らない理由は realFS のコメントを参照。
func chownDirs(dirs []string, uid, gid int, lchown func(string, int, int) error) error {
	for _, d := range dirs {
		if err := lchown(d, uid, gid); err != nil {
			return fmt.Errorf("%s の所有者変更に失敗しました: %w", d, err)
		}
	}
	return nil
}

// missingDirs は dir までの経路のうち、まだ存在せず home より下にあるものを
// 浅い順に返す。home 自身とそれより上は含めない。
func missingDirs(dir, home string) []string {
	var missing []string
	h := filepath.Clean(home)
	for p := filepath.Clean(dir); p != h && underHome(h, p); p = filepath.Dir(p) {
		if _, err := os.Stat(p); err == nil {
			break
		}
		missing = append(missing, p)
	}
	slices.Reverse(missing)
	return missing
}

// chownTarget は chown すべき uid/gid を返す。ok が false なら chown しない。
//
// 実効 UID が 0 のときだけ、かつ対象が root 以外のときだけ chown する。
// 非 root では必ず失敗する呼び出しになるため判定を純粋関数に切り出しており、
// 非 root で動く CI でもこの分岐をテストできる。
func chownTarget(euid int, o owner) (int, int, bool) {
	switch {
	case euid != 0:
		return 0, 0, false
	case o.uid == 0 && o.gid == 0:
		return 0, 0, false
	default:
		return o.uid, o.gid, true
	}
}

// validUserName は SUDO_USER として受け付ける文字種かを判定する。
//
// この値は sudo -u の引数になるため、- で始まるオプション風の値や空白入りの値を
// ここで弾く。許すのは [A-Za-z0-9._-] の 1〜32 文字で、先頭の - は認めない。
// . と .. も弾く。実害はない（user.Lookup が失敗する）が、パス要素として意味を
// 持つ名前をユーザー名として通す理由がないためである。
func validUserName(name string) bool {
	if name == "" || len(name) > maxUserNameLen || name[0] == '-' || name == "." || name == ".." {
		return false
	}
	for _, c := range []byte(name) {
		ok := (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') || c == '.' || c == '_' || c == '-'
		if !ok {
			return false
		}
	}
	return true
}
