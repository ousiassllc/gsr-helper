package appconfig

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
)

// Exists は設定ファイルがあるかを返す。path が空なら DefaultPath() を使う。
//
// Load はファイルが無い場合も既定値を返すため、戻り値からは初回起動かどうかを
// 判別できない。「設定ファイルが無い場合は初回起動時に対話ウィザードを表示する」
// （FR-41）の判定に使う。呼び出し側が os.Stat(DefaultPath()) を書くと、配置先の
// 決定が confpath とここの 2 箇所に分かれる。
//
// 読めないファイルがある場合を「無い」に丸めないのは、権限の問題でウィザードが
// 毎回立ち上がり、書き込みも失敗し続ける状態を黙って作らないためである。
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
