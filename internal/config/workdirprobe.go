package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/ousiassllc/gsr-helper/internal/disk"
)

// probeTempPattern は書き込み可否を試すファイルの名前。
// ドットで始めるのは、万一残っても runner の設定ファイルとして拾われないためである。
const probeTempPattern = ".gsr-helper-probe-*"

// ProbeWorkDir は work dir の書き込み可否と残容量を実際に調べる（FR-36）。
//
// ValidateWorkDir へ渡す既定の WorkDirProbe である。検証本体と分けているのは、
// 検証を純粋関数に保つためである。
//
// **まだ無いディレクトリは、存在する最も近い親を見る。** work dir は runner の
// 追加時に作られるため、検証の時点では無いのが普通である。親も無い（ルートまで
// 遡っても見つからない）ことは通常あり得ないが、その場合はエラーを返す。
//
// 書き込み可否を statfs や mode の計算ではなく実際の作成で判定するのは、
// 読み取り専用マウント・ACL・SELinux のいずれもパーミッションビットには現れず、
// root では mode を見ても常に書けるように見えるためである。
func ProbeWorkDir(dir string) (WorkDirInfo, error) {
	base, err := nearestExisting(dir)
	if err != nil {
		return WorkDirInfo{Writable: false, AvailBytes: 0}, err
	}

	st, err := disk.FSStats(base)
	if err != nil {
		return WorkDirInfo{Writable: false, AvailBytes: 0}, fmt.Errorf("%s: %w", base, err)
	}
	return WorkDirInfo{Writable: writable(base), AvailBytes: st.AvailBytes}, nil
}

// nearestExisting は dir から遡って最初に見つかるディレクトリを返す。
func nearestExisting(dir string) (string, error) {
	for cur := filepath.Clean(dir); ; {
		fi, err := os.Stat(cur)
		switch {
		case err == nil && fi.IsDir():
			return cur, nil
		case err == nil:
			return "", fmt.Errorf("%s はディレクトリではありません: %w", cur, ErrWorkDirNotWritable)
		case !errors.Is(err, fs.ErrNotExist):
			return "", fmt.Errorf("%s の状態の取得に失敗しました: %w", cur, err)
		}

		parent := filepath.Dir(cur)
		if parent == cur {
			return "", fmt.Errorf("%s の親が見つかりません: %w", dir, ErrWorkDirNotWritable)
		}
		cur = parent
	}
}

// writable は dir に一時ファイルを作れるかを試す。
func writable(dir string) bool {
	f, err := os.CreateTemp(dir, probeTempPattern)
	if err != nil {
		return false
	}
	name := f.Name()
	_ = f.Close()
	_ = os.Remove(name)

	return true
}
