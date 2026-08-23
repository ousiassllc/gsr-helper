package tarball

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"slices"
	"strings"
)

// MaxTotalBytes は 1 回の展開で書き出せる合計バイト数の上限（2 GiB）。
//
// 圧縮された小さな tarball がディスクを埋め尽くす（decompression bomb）のを防ぐ。
// runner 本体は展開後で 300MB 程度であり、正規の tarball には十分な余裕がある。
const MaxTotalBytes int64 = 2 << 30

// MaxEntryBytes は 1 エントリを展開できるバイト数の上限（1 GiB）。
const MaxEntryBytes int64 = 1 << 30

// extractDirMode は展開先が存在しない場合に作るときのパーミッション。
const extractDirMode fs.FileMode = 0o750

// implicitDirMode は tar にディレクトリのエントリが無いまま配下のファイルが
// 現れた場合に補うパーミッション。
const implicitDirMode fs.FileMode = 0o755

// ownerRWX は展開の途中で配下へ書き込むために必ず立てておくビット。
const ownerRWX fs.FileMode = 0o700

// limits は 1 回の展開に許すサイズ。テストから小さい値を渡せるよう型にしてある。
type limits struct {
	// total は合計の上限。
	total int64
	// entry は 1 エントリあたりの上限。
	entry int64
}

// pendingDir は展開の最後にパーミッションを戻すディレクトリ。
type pendingDir struct {
	// name は展開先からの相対パス。
	name string
	// perm は tar のヘッダが指定したパーミッション。
	perm fs.FileMode
}

// Extract は tar.gz を destDir へ展開する。
//
// keep に載っている名前（ディレクトリ直下の相対パス先頭要素）は上書きしない。
// バージョン更新では PreservedNames() を渡す（FR-21）。
//
// tar の中身は信用しない。展開先を os.Root で根に固定した上で、絶対パス・".." を
// 含むエントリ・展開先の外を指すリンクを拒否し、展開量にも上限を設ける。
//
// ctx はエントリの境界で見る。runner 本体は展開後で 300MB 程度あり、終了要求から
// 完了まで待たせないために途中で打ち切れるようにしてある
// （docs/architecture/security.md「context でキャンセルできる」）。
func Extract(ctx context.Context, src, destDir string, keep []string) error {
	return extractTo(ctx, src, destDir, keep, limits{total: MaxTotalBytes, entry: MaxEntryBytes})
}

// extractTo は上限を指定して展開する。Extract の実体。
func extractTo(ctx context.Context, src, destDir string, keep []string, lim limits) error {
	if strings.TrimSpace(destDir) == "" {
		return errors.New("展開先ディレクトリが空です")
	}

	f, err := openForRead(src)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("%s を gzip として読めませんでした: %w", src, err)
	}
	defer func() { _ = gz.Close() }()

	if err := os.MkdirAll(destDir, extractDirMode); err != nil {
		return fmt.Errorf("展開先 %s の作成に失敗しました: %w", destDir, err)
	}
	// 以降のファイル操作はすべてこの根の内側に閉じる。経路の途中がシンボリック
	// リンクに差し替えられていても、外側には 1 バイトも書けない。
	root, err := os.OpenRoot(destDir)
	if err != nil {
		return fmt.Errorf("展開先 %s を開けませんでした: %w", destDir, err)
	}
	defer func() { _ = root.Close() }()

	return extractAll(ctx, tar.NewReader(gz), root, keep, lim)
}

// extractAll は tar のエントリを順に展開する。
func extractAll(ctx context.Context, tr *tar.Reader, root *os.Root, keep []string, lim limits) error {
	var (
		written int64
		dirs    []pendingDir
	)
	links := linkSet{}

	for {
		// 打ち切りはエントリの境界で見る。書きかけのファイルを残さずに済む。
		if err := ctx.Err(); err != nil {
			return err
		}

		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return restoreDirModes(root, dirs)
		}
		if err != nil {
			return fmt.Errorf("tar の読み取りに失敗しました: %w", err)
		}

		name, err := safeName(hdr.Name)
		if err != nil {
			return err
		}
		// 空はアーカイブの根そのもの。
		if name == "" {
			continue
		}
		if err := links.checkKeep(hdr, name, keep); err != nil {
			return err
		}
		// keep は runner 自身の状態なので触らない。判定は見かけの名前ではなく、
		// このアーカイブが作ったリンクを辿った先で行う（FR-21）。
		if isPreserved(links.resolve(name), keep) {
			continue
		}

		n, err := extractEntry(tr, root, hdr, name, min(lim.entry, lim.total-written), &dirs)
		if err != nil {
			return err
		}
		links.remember(hdr, name)
		written += n
	}
}

// extractEntry はエントリ 1 件を展開し、書き出したバイト数を返す。
//
// 通常ファイル・ディレクトリ・シンボリックリンク・ハードリンクだけを扱う。
// デバイスファイルや FIFO は runner の tarball に現れず、root 権限で作らせる
// 理由も無いため読み飛ばす。
func extractEntry(
	tr *tar.Reader,
	root *os.Root,
	hdr *tar.Header,
	name string,
	allowed int64,
	dirs *[]pendingDir,
) (int64, error) {
	perm := hdr.FileInfo().Mode().Perm()

	switch hdr.Typeflag {
	case tar.TypeDir:
		return 0, makeDir(root, name, perm, dirs)
	case tar.TypeReg:
		return writeRegular(tr, root, hdr, name, allowed)
	case tar.TypeSymlink:
		return 0, writeSymlink(root, name, hdr.Linkname)
	case tar.TypeLink:
		return 0, writeHardLink(root, name, hdr.Linkname)
	default:
		return 0, nil
	}
}

// makeDir はディレクトリを作り、最後にパーミッションを戻す対象として控える。
//
// 作る時点では所有者の rwx を必ず立てる。ヘッダが 0o555 のような値でも、配下の
// エントリを書き終えるまでは書き込めなければならない。
func makeDir(root *os.Root, name string, perm fs.FileMode, dirs *[]pendingDir) error {
	if err := root.MkdirAll(name, perm|ownerRWX); err != nil {
		return fmt.Errorf("ディレクトリ %s の作成に失敗しました: %w", name, err)
	}
	*dirs = append(*dirs, pendingDir{name: name, perm: perm})
	return nil
}

// restoreDirModes は控えておいたディレクトリのパーミッションを戻す。
//
// tar は親を先に並べるため、逆順に適用して深いものから戻す。親から先に権限を
// 落とすと、その配下を辿れなくなって子の設定に失敗する。
func restoreDirModes(root *os.Root, dirs []pendingDir) error {
	for _, d := range slices.Backward(dirs) {
		if err := root.Chmod(d.name, d.perm); err != nil {
			return fmt.Errorf("ディレクトリ %s のパーミッション設定に失敗しました: %w", d.name, err)
		}
	}
	return nil
}

// writeRegular は通常ファイルを書き出し、書き出したバイト数を返す。
func writeRegular(tr *tar.Reader, root *os.Root, hdr *tar.Header, name string, allowed int64) (int64, error) {
	if hdr.Size > allowed {
		return 0, fmt.Errorf("%w: %s は %d バイトあり、残り %d バイトに収まりません",
			ErrTooLarge, name, hdr.Size, allowed)
	}
	if err := makeParent(root, name); err != nil {
		return 0, err
	}

	perm := hdr.FileInfo().Mode().Perm()
	f, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return 0, fmt.Errorf("ファイル %s を作成できませんでした: %w", name, err)
	}

	// 上の判定はヘッダの自己申告に基づく。実際の読み出しにも同じ上限をかけ、
	// 申告より多く流れてきても書き出す量が上限を超えないようにする。
	n, copyErr := io.Copy(f, io.LimitReader(tr, allowed))
	closeErr := f.Close()

	switch {
	case copyErr != nil:
		return n, fmt.Errorf("ファイル %s の書き出しに失敗しました: %w", name, copyErr)
	case closeErr != nil:
		return n, fmt.Errorf("ファイル %s を閉じられませんでした: %w", name, closeErr)
	}

	// O_CREATE の perm は umask に削られ、既存ファイルには適用されない。
	// config.sh などの実行ビットを落とさないよう明示的に設定し直す。
	if err := root.Chmod(name, perm); err != nil {
		return n, fmt.Errorf("ファイル %s のパーミッション設定に失敗しました: %w", name, err)
	}
	return n, nil
}

// writeSymlink はシンボリックリンクを作る。向き先が展開先の外なら拒否する。
//
// os.Root はリンクを辿る操作は弾くが、外を指すリンクを作ること自体は止めない。
// 展開後のディレクトリは他の処理も歩くため、ここで作らせない。
func writeSymlink(root *os.Root, name, linkname string) error {
	if err := checkLinkTarget(name, linkname); err != nil {
		return err
	}
	if err := makeParent(root, name); err != nil {
		return err
	}

	// 既存があると Symlink は失敗する。バージョン更新での置き換えを通すため外す。
	_ = root.Remove(name)

	if err := root.Symlink(linkname, name); err != nil {
		return fmt.Errorf("シンボリックリンク %s の作成に失敗しました: %w", name, err)
	}
	return nil
}

// writeHardLink はハードリンクを作る。リンク先はアーカイブ内の相対パスとして解釈する。
func writeHardLink(root *os.Root, name, linkname string) error {
	target, err := safeName(linkname)
	if err != nil {
		return err
	}
	if target == "" {
		return fmt.Errorf("%w: %s のハードリンク先が空です", ErrUnsafePath, name)
	}
	if err := makeParent(root, name); err != nil {
		return err
	}

	_ = root.Remove(name)

	if err := root.Link(target, name); err != nil {
		return fmt.Errorf("ハードリンク %s の作成に失敗しました: %w", name, err)
	}
	return nil
}
