package edit

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

// Loader は runner から設定ファイルの位置を決めて読む。
//
// **drop-in の根を差し替えられるようにするために構造体にしてある。** 既定の
// /etc/systemd/system へ書くテストは書けないため、テストは t.TempDir() を根に
// 与える。それ以外の状態は持たない。
type Loader struct {
	// DropInRoot は drop-in を置く根。空なら /etc/systemd/system。
	DropInRoot string
}

// EnvPath は .env の位置を返す。
func (l Loader) EnvPath(r runner.Runner) string { return filepath.Join(r.Dir, envName) }

// PathPath は .path の位置を返す。
func (l Loader) PathPath(r runner.Runner) string { return filepath.Join(r.Dir, pathName) }

// DropInPath は drop-in の位置を返す。ユニット名が無ければ空文字と偽を返す。
func (l Loader) DropInPath(r runner.Runner) (string, bool) {
	if r.UnitName == "" {
		return "", false
	}

	p, err := config.DropInPath(l.DropInRoot, r.UnitName)
	if err != nil {
		return "", false
	}
	return p, true
}

// Env は .env を読む。
func (l Loader) Env(r runner.Runner) (config.EnvFile, error) {
	return config.LoadEnv(l.EnvPath(r))
}

// PathFile は .path を読む。
func (l Loader) PathFile(r runner.Runner) (config.PathFile, error) {
	return config.LoadPathFile(l.PathPath(r))
}

// DropIn は drop-in を読む。ユニット名が無ければ空の DropIn を返す。
func (l Loader) DropIn(r runner.Runner) (config.DropIn, error) {
	p, ok := l.DropInPath(r)
	if !ok {
		return config.DropIn{Directives: nil}, nil
	}
	return config.LoadDropIn(p)
}
