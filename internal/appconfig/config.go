// Package appconfig は gsr-helper 自身の設定（YAML）の読み書きを担う。
//
// 設定ファイルはすべての項目に既定値を持ち、ファイルが存在しなくても動作する
// （初回起動を異常として扱わない）。配置先は実行ユーザー（sudo 実行時は SUDO_USER）の
// 設定ディレクトリで、root のホームには置かない。詳細は docs/architecture/security.md
// の「設定と状態ファイルの所有者に注意する」を参照。
//
// 配置先の決定と所有者・パーミッションは internal/appconfig/confpath、起動時の
// 能力判定は internal/appconfig/hostcaps にある。本パッケージはそれらの入口を
// 再公開する（alias.go）ので、呼び出し側は appconfig だけを import すればよい。
package appconfig

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/ousiassllc/gsr-helper/internal/appconfig/confpath"
)

// 既定値。設定ファイルが無い場合も項目が省略された場合もこの値になる。
const (
	defaultScanDepth       = 2
	defaultRefreshInterval = 3
	defaultDiskWarn        = 80
	defaultDiskCritical    = 90
	defaultAuditLog        = "/var/log/gsr-helper/audit.jsonl"
	defaultInstallBase     = "/opt/runners"
)

// maxConfigSize は読み込む設定ファイルの大きさの上限。
//
// 手編集の YAML がこの大きさを超えることはなく、--config /dev/zero のような
// 指定でメモリを食い潰さないための歯止めである。
const maxConfigSize = 1 << 20

// header は書き出す YAML の先頭に付けるコメント。
const header = `# gsr-helper の設定ファイル。
# 各項目は省略でき、省略した項目は既定値になる。
`

// Config は gsr-helper 自身の設定。
type Config struct {
	ScanRoots []string `yaml:"scan_roots"`
	ScanDepth int      `yaml:"scan_depth"`
	// RefreshInterval は一覧の自動更新間隔（秒）。
	// yaml.v3 は裸の 3 を time.Duration に入れると 3ns として扱うため int で持つ。
	RefreshInterval int            `yaml:"refresh_interval"`
	DiskThresholds  DiskThresholds `yaml:"disk_thresholds"`
	AuditLog        string         `yaml:"audit_log"`
	Defaults        Defaults       `yaml:"defaults"`
}

// DiskThresholds はディスク使用率の警告閾値（%）。
type DiskThresholds struct {
	Warn     int `yaml:"warn"`
	Critical int `yaml:"critical"`
}

// Defaults は runner 追加時の既定値。
type Defaults struct {
	// NamePrefix は runner 名のプレフィクス。空ならホスト名を使う。
	NamePrefix  string   `yaml:"name_prefix"`
	InstallBase string   `yaml:"install_base"`
	Labels      []string `yaml:"labels"`
	Ephemeral   bool     `yaml:"ephemeral"`
}

// Default は設定ファイルが無いときに使う既定の設定を返す。
func Default() Config {
	return Config{
		ScanRoots:       nil,
		ScanDepth:       defaultScanDepth,
		RefreshInterval: defaultRefreshInterval,
		DiskThresholds:  DiskThresholds{Warn: defaultDiskWarn, Critical: defaultDiskCritical},
		AuditLog:        defaultAuditLog,
		Defaults: Defaults{
			NamePrefix:  "",
			InstallBase: defaultInstallBase,
			Labels:      nil,
			Ephemeral:   false,
		},
	}
}

// RefreshDuration は自動更新間隔を time.Duration で返す。
func (c Config) RefreshDuration() time.Duration {
	return time.Duration(c.RefreshInterval) * time.Second
}

// Load は path の設定ファイルを読み込む。path が空なら DefaultPath() を使う。
//
// ファイルが無い場合・中身が空（コメントのみを含む）の場合は既定値を返す。
// どちらも初回起動で普通に起こる状態であり、これを異常にすると起動できなくなる。
// 未知のキーはエラーにする（手編集を想定した形式なので打ち間違いを黙って無視しない）。
func Load(path string) (Config, error) {
	path, err := pathOrDefault(path)
	if err != nil {
		return Config{}, err
	}

	b, err := readLimited(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return Default(), nil
		}
		return Config{}, fmt.Errorf("%s の読み込みに失敗しました: %w", path, err)
	}

	// 既定値で埋めた構造体にデコードする。yaml.v3 は存在しないキーを
	// ゼロ値で上書きしないため、「部分指定 = 残りは既定値」がこれで成立する。
	cfg := Default()
	dec := yaml.NewDecoder(bytes.NewReader(b))
	dec.KnownFields(true)
	if derr := dec.Decode(&cfg); derr != nil {
		// 空ファイルやコメントのみのファイルでは Decode が io.EOF を返す。
		if errors.Is(derr, io.EOF) {
			return Default(), nil
		}
		return Config{}, fmt.Errorf("%s の解析に失敗しました: %w", path, describeYAMLError(derr))
	}
	// Decode は先頭のドキュメントだけを読む。--- で区切った 2 本目を黙って捨てるのは
	// 「未知のキーを黙って無視しない」という本パッケージの方針と食い違うので弾く。
	if !singleDocument(dec) {
		return Config{}, fmt.Errorf("%s には YAML ドキュメントを 1 つだけ書いてください", path)
	}

	cfg, err = normalize(cfg)
	if err != nil {
		return Config{}, fmt.Errorf("%s の内容が不正です: %w", path, err)
	}
	return cfg, nil
}

// singleDocument は 2 本目以降に中身のあるドキュメントが無いかを返す。
//
// 末尾に --- だけが残っているファイルは受け付ける。区切りは 2 本目（中身の無い
// null ドキュメント）を作るが、捨てるものが無いので「黙って捨てない」方針に
// 反しない。手編集を前提にした形式（non-functional.md）では区切りだけが
// 残る状態が普通に起こるため、これを起動失敗にはしない。
//
// 中身の判定を Config ではなく yaml.Node で行うのは、Config へデコードすると
// null ドキュメントと「キーを 1 つも持たない本物の 2 本目」が同じ結果になり、
// エラーを返さない Decode を区切りだけの場合と区別できないためである。
func singleDocument(dec *yaml.Decoder) bool {
	for {
		var doc yaml.Node
		err := dec.Decode(&doc)
		if errors.Is(err, io.EOF) {
			return true
		}
		// 壊れた 2 本目も「1 つだけ書く」規則の違反として同じ扱いにする。
		if err != nil || !emptyDocument(&doc) {
			return false
		}
	}
}

// emptyDocument は中身の無いドキュメントかを返す。
//
// 末尾の区切りだけが作る null ドキュメントは、Decode がゼロ値ではなく
// 子を 1 つ持つ DocumentNode を返す（子は !!null のスカラ）ため、
// yaml.Node.IsZero では判別できない。
func emptyDocument(doc *yaml.Node) bool {
	if doc.IsZero() {
		return true
	}
	if doc.Kind != yaml.DocumentNode {
		return false
	}
	for _, child := range doc.Content {
		if child.Tag != "!!null" {
			return false
		}
	}
	return true
}

// pathOrDefault は空のパスを既定の配置先で埋める。Load と Exists が通す。
func pathOrDefault(path string) (string, error) {
	if path == "" {
		return DefaultPath()
	}
	return filepath.Clean(path), nil
}

// readLimited は maxConfigSize までを読み、それを超える入力はエラーにする。
func readLimited(path string) ([]byte, error) {
	//nolint:gosec // path は利用者が指定した設定ファイルの位置そのもの（読み込みが本関数の目的）。Clean 済みで、内容は Config の形にのみデコードする。
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	b, err := io.ReadAll(io.LimitReader(f, maxConfigSize+1))
	if err != nil {
		return nil, err
	}
	if len(b) > maxConfigSize {
		return nil, fmt.Errorf("設定ファイルが大きすぎます（上限 %d バイト）", maxConfigSize)
	}
	return b, nil
}

// Exists は設定ファイルがあるかを返す。path が空なら DefaultPath() を使う。
//
// Load はファイルが無い場合も既定値を返すため、呼び出し側が初回起動を判別できない。
// 「設定ファイルが無い場合は初回起動時に対話ウィザードを表示する」（FR-41）の
// 判定に使う。呼び出し側が os.Stat(DefaultPath()) を書くとパス決定が二重化する。
func Exists(path string) (bool, error) {
	path, err := pathOrDefault(path)
	if err != nil {
		return false, err
	}
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("%s の確認に失敗しました: %w", path, err)
	}
	return true, nil
}

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
