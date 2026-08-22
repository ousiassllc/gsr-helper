package audit

import (
	"errors"
	"fmt"
	"os"
	"syscall"
)

// rotationAwareWriter は監査ログのファイルを開き直しながら追記する io.WriteCloser。
//
// ローテーションは logrotate に委ねている（docs/architecture/data-model.md）が、
// logrotate 既定の create モードはファイルを rename して新しいファイルを作る。
// プロセス全体で fd を握り続けると、rename 後のレコードはリンクの切れた inode に
// 書かれ、誰にも読めない場所へ無音で消える。書き込み前にパスの inode を確認し、
// fd の指す inode とずれていたら開き直すことで、この監査ログの欠落を防ぐ。
//
// レコードごとに stat を 1 回行うが、記録の頻度は 3 秒ポーリング相当なので
// コストは問題にならない。
//
// 排他は持たない。Logger.Write / Logger.Close は l.mu を保持したまま Write /
// Close を呼ぶため、開き直しはすでに直列化されている。
type rotationAwareWriter struct {
	path   string
	f      *os.File
	dev    uint64
	ino    uint64
	closed bool
}

// newRotationAwareWriter は開かれた f を path の追記先として包む。
// f の所有権は writer に移り、Close で閉じられる。
func newRotationAwareWriter(path string, f *os.File) (*rotationAwareWriter, error) {
	dev, ino, err := fileID(f)
	if err != nil {
		return nil, fmt.Errorf("%s の状態の取得に失敗しました: %w", path, err)
	}
	return &rotationAwareWriter{path: path, f: f, dev: dev, ino: ino}, nil
}

// Write は必要なら開き直したうえで p を追記する。
func (w *rotationAwareWriter) Write(p []byte) (int, error) {
	if w.closed {
		return 0, fmt.Errorf("%s は既に閉じられています: %w", w.path, os.ErrClosed)
	}
	if err := w.reopenIfReplaced(); err != nil {
		return 0, err
	}
	return w.f.Write(p)
}

// Close は開いているファイルを閉じる。二重呼び出しは no-op。
func (w *rotationAwareWriter) Close() error {
	if w.closed {
		return nil
	}
	w.closed = true
	return w.f.Close()
}

// reopenIfReplaced は path が fd と同じ inode を指していなければ開き直す。
//
// 開き直しには Open と同じ openAuditFile を使う。ローテーション直後は攻撃者が
// 新しい audit.jsonl を先置きできる隙間ができるため、初回と同じ検証を通す必要が
// ある。新しい fd を得てから古い fd を閉じるのは、開き直しに失敗したときに
// writer を壊さないためである。
func (w *rotationAwareWriter) reopenIfReplaced() error {
	if fi, err := os.Lstat(w.path); err == nil {
		if st, ok := fi.Sys().(*syscall.Stat_t); ok && uint64(st.Dev) == w.dev && uint64(st.Ino) == w.ino {
			return nil
		}
	}

	f, err := openAuditFile(w.path)
	if err != nil {
		return err
	}
	dev, ino, err := fileID(f)
	if err != nil {
		_ = f.Close()
		return fmt.Errorf("%s の状態の取得に失敗しました: %w", w.path, err)
	}

	_ = w.f.Close()
	w.f, w.dev, w.ino = f, dev, ino
	return nil
}

// fileID は fd が指す inode の識別子（デバイス番号と inode 番号）を返す。
// パスではなく fd から取ることで、比較対象が確実に書き込み先の実体になる。
func fileID(f *os.File) (dev, ino uint64, err error) {
	fi, err := f.Stat()
	if err != nil {
		return 0, 0, err
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, 0, errors.New("inode 情報を取得できません")
	}
	return uint64(st.Dev), uint64(st.Ino), nil
}
