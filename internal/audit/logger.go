package audit

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

// Logger は Record を JSON Lines で追記する。複数の goroutine から同時に
// 呼んでも 1 レコードが 1 行として書かれる。
//
// mutex を持つため、値としてコピーせず必ずポインタで扱う。
type Logger struct {
	mu       sync.Mutex
	w        io.Writer
	closer   io.Closer
	now      func() time.Time
	uid      int
	sudoUser string
}

// Option は Logger の生成時の設定。
type Option func(*Logger)

// WithClock は ts に使う時刻の取得元を差し替える。テストで時刻を固定するために置いている。
//
// 渡した関数は Logger のロックを保持したまま呼ばれる。行の順序と ts の順序を
// 一致させるため時刻取得と書き込みを同じクリティカルセクションに置いており、
// 関数の中から Logger を呼び戻すとデッドロックする。
func WithClock(now func() time.Time) Option {
	return func(l *Logger) {
		if now != nil {
			l.now = now
		}
	}
}

// WithIdentity は uid / sudo_user を明示する。
// 既定は os.Geteuid() と os.Getenv("SUDO_USER")。
func WithIdentity(uid int, sudoUser string) Option {
	return func(l *Logger) {
		l.uid = uid
		l.sudoUser = sudoUser
	}
}

// New は w に追記する Logger を返す。w を閉じるのは呼び出し側の責務。
func New(w io.Writer, opts ...Option) *Logger {
	l := &Logger{
		w:        w,
		now:      time.Now,
		uid:      os.Geteuid(),
		sudoUser: os.Getenv("SUDO_USER"),
	}
	for _, opt := range opts {
		opt(l)
	}
	return l
}

// Discard は書き込みを行わない Logger を返す。
//
// 監査ログの出力先が未設定でも呼び出し側が nil 判定を書かずに済むようにする。
// nil の *Logger も同じく no-op として扱う。
func Discard() *Logger {
	return &Logger{now: time.Now}
}

// Write は rec を 1 行として追記する。
//
// ts / uid / sudo_user は Logger が持つ値で上書きする。プロセス内で不変な値を
// 呼び出し側に毎回埋めさせない。
//
// bufio は使わず、mutex の下で 1 レコードを 1 回の Write で書き切る。
// 途中で切れたレコードが残ると以降の行まで読めなくなるため、O_APPEND への
// 1 回の write が原子的であることに頼って行の分割を避けている。
//
// fsync は行わない。原子的な append で「書けた行は以降も読める」ことは満たしており、
// 3 秒ポーリングで生じる記録量に対してレコードごとの fsync は見合わないためである。
func (l *Logger) Write(rec Record) error {
	if l == nil || l.w == nil {
		return nil
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	// 時刻の取得を書き込みと同じロック下に置く。ロックの外で取ると、同時に
	// Write した goroutine 同士で ts の順序と行の順序が入れ替わり、監査ログを
	// 時系列として読めなくなる。
	rec.TS = Timestamp(l.now())
	rec.UID = l.uid
	rec.SudoUser = l.sudoUser

	b, err := json.Marshal(rec)
	if err != nil {
		return fmt.Errorf("監査レコードの JSON 変換に失敗しました: %w", err)
	}
	b = append(b, '\n')

	if _, err := l.w.Write(b); err != nil {
		return fmt.Errorf("監査ログの書き込みに失敗しました: %w", err)
	}
	return nil
}

// Close は Open が開いたファイルを閉じる。New に渡した Writer は所有者が
// 呼び出し側のため閉じない。二重呼び出しは no-op。
func (l *Logger) Close() error {
	if l == nil {
		return nil
	}

	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closer == nil {
		return nil
	}
	c := l.closer
	l.closer = nil
	if err := c.Close(); err != nil {
		return fmt.Errorf("監査ログのクローズに失敗しました: %w", err)
	}
	return nil
}
