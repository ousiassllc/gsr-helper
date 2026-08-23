package tarball

import (
	"archive/tar"
	"context"
	"crypto/rand"
	"fmt"
	"os"
	"path"
	"strings"
)

// stagePrefix は展開用の一時ディレクトリに付ける接頭辞。強制終了で置き去りに
// なったものを次回の展開で掃除するための目印も兼ねる。
const stagePrefix = ".gsr-stage-"

// stage は展開先の中に作る一時展開領域。
//
// tar を最終位置へ直接書くと、io.Copy の途中で強制終了された場合に切り詰められた
// ファイルが残り、runner が新旧混在の壊れた状態になる（終了要求を受けた側は展開の
// 完了を待たない）。いったん一時領域へ展開して最後に rename で移せば、1 ファイル
// ずつは「完全に旧」か「完全に新」のどちらかにしかならない。一時領域を展開先の
// 中に作るのは、rename を同一ファイルシステム内の原子的な操作にするためである。
type stage struct {
	// dest は展開先の根。移し替えと後始末はこの根の内側で行う。
	dest *os.Root
	// name は展開先から見た一時領域の名前。
	name string
	// root は一時領域を根に固定したもの。展開はこの内側だけで行う。
	root *os.Root
}

// extractStaged は tar を一時領域へ展開してから展開先へ移し替える。
// 途中で失敗した場合は展開先の既存ファイルに触れないまま一時領域ごと捨てる。
func extractStaged(ctx context.Context, tr *tar.Reader, root *os.Root, keep []string, lim limits) error {
	st, err := newStage(root)
	if err != nil {
		return err
	}
	// 成功でも失敗でも一時領域は残さない。失敗経路では best-effort でよい。
	defer st.cleanup()

	dirs, err := extractAll(ctx, tr, st.root, keep, lim)
	if err != nil {
		return err
	}
	if err := st.promote("", keep); err != nil {
		return err
	}
	// ディレクトリのパーミッションは移し終えてから戻す。先に 0o555 のような値へ
	// 落とすと、その配下を一時領域から動かせなくなる。
	return restoreDirModes(root, dirs)
}

// newStage は展開先の中に一時領域を作る。
func newStage(dest *os.Root) (*stage, error) {
	if err := sweepStaleStages(dest); err != nil {
		return nil, err
	}
	// rand.Text の 26 文字は、掃除をすり抜けた古い一時領域や tar 側の同名エントリと
	// ぶつからないだけの幅がある。所有者だけが読み書きできれば足りる。
	name := stagePrefix + strings.ToLower(rand.Text())
	if err := dest.Mkdir(name, ownerRWX); err != nil {
		return nil, fmt.Errorf("一時展開先 %s の作成に失敗しました: %w", name, err)
	}
	root, err := dest.OpenRoot(name)
	if err != nil {
		_ = dest.RemoveAll(name)
		return nil, fmt.Errorf("一時展開先 %s を開けませんでした: %w", name, err)
	}
	return &stage{dest: dest, name: name, root: root}, nil
}

// sweepStaleStages は前回の強制終了で置き去りになった一時領域を消す。
//
// 一時領域は展開先の中に作るため、後始末の前に落とされると残る。runner の tarball
// は展開後で 300MB 程度あり、放置するとディスクを食い潰す。
func sweepStaleStages(dest *os.Root) error {
	entries, err := readDirIn(dest, ".")
	if err != nil {
		return err
	}
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), stagePrefix) {
			continue
		}
		if err := dest.RemoveAll(e.Name()); err != nil {
			return fmt.Errorf("古い一時展開先 %s を消せませんでした: %w", e.Name(), err)
		}
	}
	return nil
}

// cleanup は一時領域を片付ける。成功経路・失敗経路のどちらからも呼ぶ。
func (st *stage) cleanup() {
	_ = st.root.Close()
	_ = st.dest.RemoveAll(st.name)
}

// promote は一時領域の rel 配下を展開先の最終位置へ移す。
//
// 展開先に同名の実体ディレクトリがある場合だけ中へ降りる。それ以外はディレクトリ
// ごと rename 一発で移すため、配下が途中の状態で見えることは無い。
func (st *stage) promote(rel string, keep []string) error {
	entries, err := readDirIn(st.root, path.Join(".", rel))
	if err != nil {
		return err
	}
	for _, e := range entries {
		name := path.Join(rel, e.Name())
		// 展開時に読み飛ばしているのでここへは来ないはずだが、保持対象を最終位置へ
		// 置くことは FR-21 違反そのものなので、移し替え側でも止めておく。
		if isPreserved(name, keep) {
			continue
		}
		// ReadDir の IsDir はシンボリックリンクを辿らない。リンクは中へ降りずに移す。
		if e.IsDir() && isRealDir(st.dest, name) {
			if err := st.promote(name, keep); err != nil {
				return err
			}
			continue
		}
		if err := st.move(name); err != nil {
			return err
		}
	}
	return nil
}

// move は一時領域の 1 件を展開先の最終位置へ rename で移す。
//
// rename は同じ種別なら既存を原子的に置き換える。ファイルとディレクトリのように
// 種別が変わるときだけ失敗するので、その場合に限り既存を外してやり直す。
func (st *stage) move(name string) error {
	from := path.Join(st.name, name)
	if err := st.dest.Rename(from, name); err == nil {
		return nil
	}
	if err := st.dest.RemoveAll(name); err != nil {
		return fmt.Errorf("既存の %s を外せませんでした: %w", name, err)
	}
	if err := st.dest.Rename(from, name); err != nil {
		return fmt.Errorf("%s の配置に失敗しました: %w", name, err)
	}
	return nil
}

// isRealDir は root の中の name がリンクではない実体のディレクトリかを返す。
// リンクを辿って判定すると、展開先に既にあるリンクの先へ中身を書き込んでしまう。
func isRealDir(root *os.Root, name string) bool {
	fi, err := root.Lstat(name)
	return err == nil && fi.IsDir()
}

// readDirIn は root の中の dir の一覧を返す。
func readDirIn(root *os.Root, dir string) ([]os.DirEntry, error) {
	f, err := root.Open(dir)
	if err != nil {
		return nil, fmt.Errorf("%s を開けませんでした: %w", dir, err)
	}
	defer func() { _ = f.Close() }()

	entries, err := f.ReadDir(-1)
	if err != nil {
		return nil, fmt.Errorf("%s の読み取りに失敗しました: %w", dir, err)
	}
	return entries, nil
}
