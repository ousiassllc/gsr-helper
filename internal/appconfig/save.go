package appconfig

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/ousiassllc/gsr-helper/internal/appconfig/confpath"
)

// 設定ファイルの書き込み。
//
// 読み取り（config.go）と分けているのは、**気にしているものが違う**ためである。
// あちらが相手にするのは YAML の解釈（未知のキー・複数ドキュメント・大きすぎる
// ファイル）で、こちらが相手にするのはファイルシステム——一時ファイルと rename に
// よる置き換え、所有者、security.md が定めるパーミッション——である。1 ファイル
// 300 行の上限（docs/ui/atomic-design.md）に収める際の切れ目もここになる
// （Issue #113）。

// Save は cfg を path に書き出す。path が空なら DefaultPath() を使う。
//
// 一時ファイルへ書いてから rename する。os.WriteFile では perm が新規作成時にしか
// 効かないため、root が一度 644 で作ったファイルが 600 に直らず、所有者も root の
// ままになる（次に非 root で起動したときに読めない）。これは security.md が挙げる
// 事故そのものなので、毎回 600・SUDO_USER 所有で作り直せる経路にしている。
//
// 出力は固定のヘッダコメント + yaml.Marshal で、利用者が書いたコメントは失われる。
// 非機能要件が求めるのは「人が手編集できる形式」であってコメントの保持ではなく、
// コメント保持が要件なのは runner の .env 側（internal/config）である。
func Save(cfg Config, path string) error {
	cfg, err := normalize(cfg)
	if err != nil {
		return err
	}
	o, err := confpath.Resolve()
	if err != nil {
		return err
	}
	if path == "" {
		path = o.ConfigPath()
	}
	path = filepath.Clean(path)

	dir := filepath.Dir(path)
	if err := o.MkdirOwned(dir); err != nil {
		return err
	}
	b, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("設定の YAML 変換に失敗しました: %w", err)
	}
	return writeAtomic(path, dir, append([]byte(header), b...), o)
}

// writeAtomic は同じディレクトリに一時ファイルを作って書き、rename で置き換える。
//
// 一時ファイルを同一ディレクトリに作るのは、別ファイルシステム間の rename が
// 失敗するのを避けるため。名前をドットで始めるのは、残骸が設定ファイルとして
// 誤認されないようにするため。
//
// rename 後に親ディレクトリの fsync はしない。ここで守るのは「中途半端な内容の
// 設定ファイルを残さない」ことまでで、電源断で rename 自体が失われることは
// 許容する（失われても既定値で起動でき、利用者が書き直せば済む）。
func writeAtomic(path, dir string, b []byte, o confpath.Owner) error {
	f, err := os.CreateTemp(dir, ".config.yaml.*")
	if err != nil {
		return fmt.Errorf("%s への一時ファイルの作成に失敗しました: %w", dir, err)
	}
	tmp := f.Name()
	defer func() { _ = os.Remove(tmp) }()

	if err := writeTemp(f, b, o); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("%s への書き込みに失敗しました: %w", path, err)
	}
	return nil
}

// writeTemp は一時ファイルにパーミッション・所有者・内容を設定する。
func writeTemp(f *os.File, b []byte, o confpath.Owner) error {
	err := fillTemp(f, b, o)
	// Close の戻り値も確認する。書き込みの失敗は Close の時点で初めて現れることがある。
	if cerr := f.Close(); err == nil && cerr != nil {
		err = fmt.Errorf("一時ファイルのクローズに失敗しました: %w", cerr)
	}
	return err
}

// fillTemp は open 済みの一時ファイルを目的の状態にする。
func fillTemp(f *os.File, b []byte, o confpath.Owner) error {
	// os.CreateTemp は 0600 で作るが umask の影響を受けるため明示的に設定する。
	if err := f.Chmod(confpath.FileMode); err != nil {
		return fmt.Errorf("一時ファイルのパーミッション設定に失敗しました: %w", err)
	}
	if uid, gid, ok := o.ChownTarget(os.Geteuid()); ok {
		if err := f.Chown(uid, gid); err != nil {
			return fmt.Errorf("一時ファイルの所有者変更に失敗しました: %w", err)
		}
	}
	if _, err := f.Write(b); err != nil {
		return fmt.Errorf("一時ファイルへの書き込みに失敗しました: %w", err)
	}
	if err := f.Sync(); err != nil {
		return fmt.Errorf("一時ファイルの同期に失敗しました: %w", err)
	}
	return nil
}
