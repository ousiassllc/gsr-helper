// Package config は runner 側の設定ファイル（.env / .path / systemd drop-in）の
// 読み書きと、書き込み前の差分・バックアップ・入力検証を提供する（FR-35〜FR-40）。
//
// 実体は下位パッケージに分かれており、ここはそれらを仕様書の名前
// （docs/components/overview.md の internal/config の表）でまとめて見せる層である。
// 分けているのは行数上限（1 ファイル 300 行 / 1 ディレクトリ 2000 行）のためだが、
// 責務の切れ目とも一致している。
//
//   - config/fileio  … どう読み書きするか（シンボリックリンクの拒否・原子的な
//     置き換え・所有者とパーミッションの引き継ぎ・バックアップ）
//   - config/envfile … .env と .path の表現
//   - config/dropin  … systemd drop-in の表現と配置
//
// **ラベルと runner group はこのパッケージの対象ではない。** どちらも GitHub 側の
// 値であり、変更は internal/gh の API 呼び出しで行う（再起動も要らない）。ここが
// 持つのは入力検証だけである。
//
// ドメイン層なので internal/ui を import せず、外部コマンドも起動しない。
// drop-in を置いたあとの systemctl daemon-reload は呼び出し側（internal/svc）の
// 責務である。
package config

import (
	"github.com/ousiassllc/gsr-helper/internal/config/dropin"
	"github.com/ousiassllc/gsr-helper/internal/config/envfile"
	"github.com/ousiassllc/gsr-helper/internal/config/fileio"
)

// 仕様書の名前で下位パッケージの型を見せる別名。
type (
	// EnvFile は .env の中身。全行を順序どおり保持する。
	EnvFile = envfile.File
	// PathFile は .path の中身。
	PathFile = envfile.PathFile
	// DropIn は systemd drop-in の中身。
	DropIn = dropin.DropIn
	// Directive は drop-in の [Service] セクションの 1 行。
	Directive = dropin.Directive
)

// BackupSuffix はバックアップファイルに付ける拡張子（.bak）。
const BackupSuffix = fileio.BackupSuffix

// LoadEnv は path の .env を読む。ファイルが無ければ空の EnvFile を返す。
func LoadEnv(path string) (EnvFile, error) { return envfile.Load(path) }

// SaveEnv は f を path へ書き出す。既存ファイルの所有者とパーミッションを引き継ぐ。
func SaveEnv(f EnvFile, path string) error { return envfile.Save(f, path) }

// LoadPathFile は path の .path を読む。ファイルが無ければ空の PathFile を返す。
func LoadPathFile(path string) (PathFile, error) { return envfile.LoadPath(path) }

// SavePathFile は p を path へ書き出す。
func SavePathFile(p PathFile, path string) error { return envfile.SavePath(p, path) }

// LoadDropIn は path の drop-in を読む。ファイルが無ければ空の DropIn を返す。
func LoadDropIn(path string) (DropIn, error) { return dropin.Load(path) }

// SaveDropIn は d を path へ書き出す。親ディレクトリが無ければ作る。
//
// 反映には systemctl daemon-reload が要る（FR-35 の反映方法の表）。
func SaveDropIn(d DropIn, path string) error { return dropin.Save(d, path) }

// DropInPath は unit の drop-in ファイルのパスを返す。
// root が空なら /etc/systemd/system を使う。
func DropInPath(root, unit string) (string, error) { return dropin.Path(root, unit) }

// Backup は path のバックアップを <path>.bak として作る（FR-38）。
// 元ファイルの所有者とパーミッションを引き継ぐ。
func Backup(path string) error { return fileio.Backup(path) }
