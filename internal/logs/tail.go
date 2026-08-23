package logs

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/fsnotify/fsnotify"
)

// ログファイルのライブテール（FR-24）を置く。

const (
	// TailInitialBytes は追従を始めるときに遡って読む最大バイト数。
	//
	// 先頭から読むと、数百 MB に育った `Runner_*.log` を開いた瞬間に全行が
	// チャネルへ流れ、UI が受け切るまで固まる。末尾の一定量だけを出せば
	// 「今何が起きているか」は読めるので、遡る量に上限を置く。
	TailInitialBytes = 256 << 10
	// maxLineBytes は 1 行として送る最大バイト数。超えた分は捨てる。
	//
	// 改行を含まない出力（バイナリが混ざったログ）で 1 行の組み立てが
	// 際限なく伸びるのを防ぐ。表示できる幅をはるかに超えるので、捨てても
	// 画面に出る内容は変わらない。
	maxLineBytes = 64 << 10
	// readBufBytes は読み出しのバッファ。maxLineBytes より小さくてよい
	// （足りない分は ErrBufferFull を挟んで繰り返し読む）。
	readBufBytes = 64 << 10
)

// Tail は path のログを追従し、行を out へ送る（FR-24）。
//
// **戻るときに out を閉じる。** 受け手（UI）は閉じたことで購読の終わりを知り、
// 読み直しの Cmd を発行し続けずに済む。したがって out はこの関数専用に作ること。
//
// ctx がキャンセルされたら nil を返す。畳むのは利用者がタブを離れたときと
// アプリの終了時であり（page.DeactivateMsg / page.ShutdownMsg）、どちらも異常では
// ないためである。
//
// **監視するのはファイルではなく親ディレクトリである。** runner はログを
// 入れ替える（新しいファイルを作る）ことがあり、ファイル自身に張った監視は
// 古い inode に残って以後の追記に反応しない。ディレクトリを見て自分の
// ファイル名の変化だけを拾えば、入れ替えのあとも追従が続く。
//
// **同じ名前で作り直されたら開き直す。** 監視をディレクトリへ張っても、開いた
// ファイルハンドルは古い inode を指したままである。unlink された inode には
// もう誰も書かないので、開き直さなければ追記は永久に届かない。しかも f.Stat()
// が返すのも古い inode のサイズなので、emit の切り詰め検知（サイズが縮んだら
// 先頭へ戻る）も働かず、追従は恒久的に止まる。そこで自分のファイル名に対する
// Create を見たら開き直し、読み出し位置を取り直す（reopen）。**inode 番号の
// 照合はしない。** 同名で作り直されたのなら読み直すのが正しく、作り直しで
// なかった（既存ファイルへの O_CREAT など）としても切り詰めと同じ結果にしか
// ならないため、素直に開き直すほうが単純で確実である。
func Tail(ctx context.Context, path string, out chan<- Line) error {
	defer close(out)

	f, err := os.Open(path) //nolint:gosec // G304 表示対象のログは利用者が一覧から選ぶ。パスは runner の _diag 配下を List が列挙したもの。
	if err != nil {
		return fmt.Errorf("%s を開けません: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	w, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("%s の監視を開始できません: %w", path, err)
	}
	defer func() { _ = w.Close() }()
	if err := w.Add(filepath.Dir(path)); err != nil {
		return fmt.Errorf("%s の監視を開始できません: %w", filepath.Dir(path), err)
	}

	off, err := seekTail(f)
	if err != nil {
		return err
	}
	off, err = emit(ctx, f, off, out)
	if err != nil {
		return err
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case ev, ok := <-w.Events:
			if !ok {
				return nil
			}
			if ev.Name != path {
				continue
			}
			if ev.Has(fsnotify.Create) {
				f, off = reopen(f, path, off)
			}
			off, err = emit(ctx, f, off, out)
			if err != nil {
				return err
			}
		case werr, ok := <-w.Errors:
			if !ok {
				return nil
			}
			return fmt.Errorf("%s の監視に失敗しました: %w", path, werr)
		}
	}
}

// reopen は path を開き直し、新しいファイルと読み出し位置を返す。
//
// **開き直せなければ、いまのファイルと位置をそのまま返す。** 作成の通知が届いてから
// 開くまでにはわずかな隙があり（作り直しの途中でもう一度差し替わる、権限が整うのが
// 一瞬遅れる）、そこでエラーを返して追従を終えると、利用者には理由の分からない
// 打ち切りに見える。作り直しが続いていれば次のイベントでまた開き直せるので、
// 1 回の失敗は読み飛ばして追従を保つほうが縮退として妥当である。
//
// **作り直されたファイルも、初回と同じく末尾からしか読まない**（先頭に戻さず
// seekTail を通す）。理由は TailInitialBytes の doc と同じで、作り直しの直後に
// 大きいファイルが置かれること（ローテートで退避していたものを書き戻した、
// runner が過去ぶんをまとめて出力した）は普通に起きる。そこで先頭から読むと
// 数百 MB ぶんの全行がチャネルへ流れ、UI が受け切るまで固まる。上限を初回だけに
// 効かせても、追従している間に一度でも作り直されれば同じ事故が起きるので、
// 遡る量の上限はこちらの経路にも同じように掛ける。
//
// **seekTail に失敗したら先頭（0）から読む。** 開き直しに失敗しても追従を畳まない
// という上の縮退方針と揃えている。位置を決められないという理由で追従を打ち切ると
// 画面が理由なく止まるが、先頭から読めば最悪でも「読み過ぎる」だけで、以後の
// 追記にはきちんと追従できる。打ち切るより読み過ぎるほうがましである。
func reopen(f *os.File, path string, off int64) (*os.File, int64) {
	nf, err := os.Open(path) //nolint:gosec // G304 表示対象のログは利用者が一覧から選ぶ。パスは runner の _diag 配下を List が列挙したもの。
	if err != nil {
		return f, off
	}
	_ = f.Close()
	noff, err := seekTail(nf)
	if err != nil {
		return nf, 0
	}
	return nf, noff
}

// seekTail は末尾から TailInitialBytes ぶん遡った読み出し位置を返す。
//
// 遡った先が行の途中になるので、その行は捨てて次の改行から読み始める。
// 途中から始まる 1 行を出すと、時刻も重大度も欠けた断片が先頭に並ぶ。
func seekTail(f *os.File) (int64, error) {
	info, err := f.Stat()
	if err != nil {
		return 0, fmt.Errorf("%s の情報を取得できません: %w", f.Name(), err)
	}
	off := info.Size() - TailInitialBytes
	if off <= 0 {
		return 0, nil
	}
	if _, err := f.Seek(off, io.SeekStart); err != nil {
		return 0, fmt.Errorf("%s を読み出せません: %w", f.Name(), err)
	}
	r := bufio.NewReaderSize(f, readBufBytes)
	_, n, complete, err := readLine(r)
	if err != nil && !errors.Is(err, io.EOF) {
		return 0, fmt.Errorf("%s を読み出せません: %w", f.Name(), err)
	}
	if !complete {
		// 遡った位置から末尾まで改行が 1 つも無い。捨てる行を決められないので
		// 先頭から読む（この形になるのは 1 行が TailInitialBytes を超える場合だけ）。
		return 0, nil
	}
	return off + n, nil
}

// emit は off から末尾までの完結した行を送り、次の読み出し位置を返す。
//
// 改行で終わっていない末尾は送らず、位置も進めない。次の追記で残りが届いてから
// 1 行として送るためである（送ってしまうと同じ行が 2 度出る）。
//
// ファイルが縮んでいたら先頭へ戻る。runner がログを切り詰めた（同じ名前で
// 書き直した）場合であり、位置を保つと存在しない領域を読み続ける。
func emit(ctx context.Context, f *os.File, off int64, out chan<- Line) (int64, error) {
	info, err := f.Stat()
	if err != nil {
		return off, fmt.Errorf("%s の情報を取得できません: %w", f.Name(), err)
	}
	if info.Size() < off {
		off = 0
	}
	if _, err := f.Seek(off, io.SeekStart); err != nil {
		return off, fmt.Errorf("%s を読み出せません: %w", f.Name(), err)
	}

	r := bufio.NewReaderSize(f, readBufBytes)
	for {
		text, n, complete, err := readLine(r)
		if err != nil && !errors.Is(err, io.EOF) {
			return off, fmt.Errorf("%s を読み出せません: %w", f.Name(), err)
		}
		if !complete {
			return off, nil
		}
		off += n
		select {
		case out <- NewLine(text):
		case <-ctx.Done():
			return off, nil
		}
	}
}

// readLine は改行までを読み、本文・消費したバイト数・行が完結したかを返す。
//
// 本文は maxLineBytes で打ち切るが、消費したバイト数は打ち切らない。読み出し位置は
// 実際に読んだぶんだけ進める必要があるためである。
func readLine(r *bufio.Reader) (text string, n int64, complete bool, err error) {
	var b strings.Builder
	for {
		chunk, rerr := r.ReadSlice('\n')
		n += int64(len(chunk))
		if room := maxLineBytes - b.Len(); room > 0 {
			b.Write(chunk[:min(len(chunk), room)])
		}
		switch {
		case rerr == nil:
			return strings.TrimRight(b.String(), "\r\n"), n, true, nil
		case errors.Is(rerr, bufio.ErrBufferFull):
			continue
		default:
			return "", n, false, rerr
		}
	}
}
