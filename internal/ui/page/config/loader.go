package config

import (
	"path/filepath"

	"github.com/ousiassllc/gsr-helper/internal/config"
	"github.com/ousiassllc/gsr-helper/internal/runner"
)

// 設定ファイルの名前。runner ディレクトリ直下に置かれる。
const (
	envName  = ".env"
	pathName = ".path"
)

// loader は runner から設定ファイルの位置を決めて読む。
//
// **drop-in の根を差し替えられるようにするために構造体にしてある。** 既定の
// /etc/systemd/system へ書くテストは書けないため、テストは t.TempDir() を根に
// 与える。それ以外の状態は持たない。
type loader struct {
	// dropInRoot は drop-in を置く根。空なら /etc/systemd/system。
	dropInRoot string
}

// envPath は .env の位置を返す。
func (l loader) envPath(r runner.Runner) string { return filepath.Join(r.Dir, envName) }

// pathPath は .path の位置を返す。
func (l loader) pathPath(r runner.Runner) string { return filepath.Join(r.Dir, pathName) }

// dropInPath は drop-in の位置を返す。ユニット名が無ければ空文字と偽を返す。
func (l loader) dropInPath(r runner.Runner) (string, bool) {
	if r.UnitName == "" {
		return "", false
	}

	p, err := config.DropInPath(l.dropInRoot, r.UnitName)
	if err != nil {
		return "", false
	}
	return p, true
}

// env は .env を読む。
func (l loader) env(r runner.Runner) (config.EnvFile, error) {
	return config.LoadEnv(l.envPath(r))
}

// pathFile は .path を読む。
func (l loader) pathFile(r runner.Runner) (config.PathFile, error) {
	return config.LoadPathFile(l.pathPath(r))
}

// dropIn は drop-in を読む。ユニット名が無ければ空の DropIn を返す。
func (l loader) dropIn(r runner.Runner) (config.DropIn, error) {
	p, ok := l.dropInPath(r)
	if !ok {
		return config.DropIn{Directives: nil}, nil
	}
	return config.LoadDropIn(p)
}
