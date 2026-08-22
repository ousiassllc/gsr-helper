// Package confpath は gsr-helper 自身の設定ファイルの配置先と、その所有者・
// パーミッションを決める。
//
// appconfig から分離しているのは次の 2 点による。
//   - 「どこに置き、誰の所有にするか」は YAML の読み書きとは独立した責務であり、
//     sudo 実行時の所有者事故（docs/architecture/security.md）を防ぐ規則が
//     ここに集まる。設定の内容を扱うコードと混ぜない。
//   - SUDO_USER の検証もここに置く。配置先の決定と能力判定（hostcaps）の
//     両方がこの値を読むため、検証を 1 か所に寄せないと「配置先は検証済みの
//     値なのに記録は生値」のようなずれが起きる。
package confpath

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
)

// 配置先の決定に読む環境変数。検証を伴う読み取りをこのパッケージに寄せるため、
// 名前も公開して呼び出し側とテストが同じ定数を使えるようにする。
const (
	// EnvSudoUser は sudo 実行時の起動ユーザーを示す環境変数。
	EnvSudoUser = "SUDO_USER"
	// EnvXDGConfigHome は設定ディレクトリを上書きする環境変数。
	EnvXDGConfigHome = "XDG_CONFIG_HOME"
)

// 設定ファイルの相対位置。生成するパスは常に gsr-helper/config.yaml で終わる。
const (
	// DirName は設定ファイルを置くディレクトリ名。
	DirName = "gsr-helper"
	// FileName は設定ファイル名。
	FileName = "config.yaml"
)

// maxUserNameLen は SUDO_USER として受け付ける長さの上限。
const maxUserNameLen = 32

// Owner は設定ファイルを所有すべきユーザー。
type Owner struct {
	name string
	home string
	uid  int
	gid  int
}

// Default は設定ファイルの既定の配置先を返す。
//
// os.UserConfigDir は使わない。HOME を読むため sudo 下では /root を指しうるうえ、
// sudoers の env_keep / always_set_home 次第で挙動が変わる。
// os/user.Lookup(SUDO_USER).HomeDir から決めればホスト設定に依らず決定的になる。
func Default() (string, error) {
	o, err := Resolve()
	if err != nil {
		return "", err
	}
	return o.ConfigPath(), nil
}

// ConfigPath は o にとっての既定の配置先を返す。
// XDG_CONFIG_HOME を読むのはここだけにして、環境変数の扱いを 1 か所に寄せる。
func (o Owner) ConfigPath() string {
	return configPath(o, os.Getenv(EnvXDGConfigHome))
}

// Resolve は実環境から所有者を解決する。
func Resolve() (Owner, error) {
	return resolveOwner(SudoUserFrom(os.Getenv), user.Lookup, user.Current, isDir)
}

// SudoUserFrom は getenv 経由で SUDO_USER を読む。文字種が不正なら空を返す。
// getenv を引数に取るのは、能力判定（hostcaps）のテストで環境を差し替えるため。
//
// 読み取りと検証をこの 1 か所に寄せる。同じ値を別々の場所で読むと検証の有無が
// 食い違い、「配置先は実行ユーザーなのに記録は生値」のようなずれが起きる。
func SudoUserFrom(getenv func(string) string) string {
	name := getenv(EnvSudoUser)
	if !validUserName(name) {
		return ""
	}
	return name
}

// resolveOwner は設定ファイルの所有者を決める。
//
// sudoUser が使えないとき（未設定・不正な文字種・存在しないユーザー・ホームが無い）は
// 実行ユーザーにフォールバックする。lookup / self / homeIsDir を引数に取るのは
// テストで差し替えるためである。
//
// ホームの有無まで見るのは、ホームを持たないユーザー（サービスアカウントなど）で
// sudo された場合に設定が保存できなくなるのを避けるためである。本ツールはホームを
// 作らない（root が作ると root 所有 0700 になり当該ユーザーが通れない）ので、
// 置けない場所を指し続けるより実行ユーザーの設定ディレクトリに倒す。
func resolveOwner(
	sudoUser string,
	lookup func(string) (*user.User, error),
	self func() (*user.User, error),
	homeIsDir func(string) bool,
) (Owner, error) {
	if validUserName(sudoUser) {
		if u, err := lookup(sudoUser); err == nil {
			if o, oerr := toOwner(u); oerr == nil && homeIsDir(o.home) {
				return o, nil
			}
		}
	}
	u, err := self()
	if err != nil {
		return Owner{}, fmt.Errorf("実行ユーザーの取得に失敗しました: %w", err)
	}
	return toOwner(u)
}

// isDir は p が存在するディレクトリかを返す。
func isDir(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}

// toOwner は os/user の値を Owner に変換する。
func toOwner(u *user.User) (Owner, error) {
	uid, err := strconv.Atoi(u.Uid)
	if err != nil {
		return Owner{}, fmt.Errorf("ユーザー %s の uid の解釈に失敗しました: %w", u.Username, err)
	}
	gid, err := strconv.Atoi(u.Gid)
	if err != nil {
		return Owner{}, fmt.Errorf("ユーザー %s の gid の解釈に失敗しました: %w", u.Username, err)
	}
	return Owner{name: u.Username, home: filepath.Clean(u.HomeDir), uid: uid, gid: gid}, nil
}

// configPath は所有者と XDG_CONFIG_HOME から設定ファイルのパスを組む純粋関数。
//
// XDG_CONFIG_HOME は「絶対パスかつ対象ユーザーのホーム配下」のときだけ尊重する。
// sudo で root の値が引き継がれた場合に root のホームへ書かないためである。
func configPath(o Owner, xdgConfigHome string) string {
	base := filepath.Join(o.home, ".config")
	if x := filepath.Clean(xdgConfigHome); filepath.IsAbs(x) && underHome(o.home, x) {
		base = x
	}
	return filepath.Join(base, DirName, FileName)
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

// SudoUser は検証済みの SUDO_USER を返す。文字種が不正なら空文字を返す。
//
// 設定の配置先を決める値であり、監査ログの記録者や sudo -u の引数にもなる。
// 生の環境変数を各所で読み直すと検証の有無が食い違うため、読み取りはここに寄せる。
func SudoUser() string { return SudoUserFrom(os.Getenv) }
