package limit

// Buffer は末尾の上限バイトぶんだけを保持する書き込み先。
// NewBuffer で作る（上限を後から変えられないようにしてある）。
type Buffer struct {
	buf   []byte
	limit int
}

// NewBuffer は末尾 limit バイトを保持する Buffer を返す。
// limit が 0 以下のときは何も保持しないが、書き込みは成功として返す（Write の理由）。
func NewBuffer(limit int) *Buffer { return &Buffer{buf: nil, limit: limit} }

// Write は末尾の limit バイトを保持し、あふれた古い先頭を捨てる。
//
// 残すのを末尾側にしているのは Head と同じ理由で、失敗の原因は標準エラー
// 出力の最後の数行に出るためである。両者の向きが食い違うと、取り込み段で末尾が
// 落ちた後に Head が末尾を探すことになり、原因の行が画面から消える。
//
// 捨てても成功として返すのは、エラーを返すと os/exec が取り込みを止めてパイプが
// 閉じ、子が書き込みエラーで死んで本来観測したい終了コードが得られなくなるため
// である。上限は「保持量」の制限であって、実行の打ち切りではない。
func (b *Buffer) Write(p []byte) (int, error) {
	switch {
	case b.limit <= 0:
		// 何も保持しない。それでも書き込みは成功として返す（上記の理由）。
	case len(p) >= b.limit:
		// この 1 回で上限が埋まるので、既存の保持内容はすべて古い。
		b.buf = append(b.buf[:0], p[len(p)-b.limit:]...)
	default:
		if over := len(b.buf) + len(p) - b.limit; over > 0 {
			// 古い先頭を捨てて詰める。伸ばしてから切るのではなく先に詰めるのは、
			// 保持量を一度も limit バイト超に膨らませないためである。
			b.buf = append(b.buf[:0], b.buf[over:]...)
		}
		b.buf = append(b.buf, p...)
	}
	return len(p), nil
}

// Bytes は保持している内容を返す。
func (b *Buffer) Bytes() []byte { return b.buf }

// String は保持している内容を文字列で返す。
func (b *Buffer) String() string { return string(b.buf) }
