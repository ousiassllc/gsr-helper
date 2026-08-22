package command

import "unicode/utf8"

const (
	// maxRecordedErrorBytes は監査レコードの error に載せる上限。
	//
	// 監査ログは 1 レコード 1 行の JSONL なので、上限を置かないと 1 回の失敗が
	// 数 MB の 1 行になり、ローテーションも grep も破綻する。
	maxRecordedErrorBytes = 4 << 10 // 4 KiB

	// maxStderrExcerptBytes は ExitError.Stderr に残す上限。
	// 画面に出す抜粋なので、原因の書かれた末尾だけあれば足りる。
	maxStderrExcerptBytes = 4 << 10 // 4 KiB

	// maxStderrCaptureBytes は標準エラー出力を取り込む上限。
	//
	// 暴走した子が標準エラー出力を吐き続けてもメモリを食い潰さないための保険。
	// 上限を超えたときに残るのは末尾（limitedBuffer が古い先頭を捨てる）で、
	// 抜粋を作る truncateHead と向きを揃えてある。
	// 標準出力に同じ上限を置かないのは、呼び出し側が stdout を解析する
	// （systemctl show / list-units）ため、切ると解析が黙って壊れるからである。
	maxStderrCaptureBytes = 1 << 20 // 1 MiB

	// elisionSuffix は末尾を切り落としたことを示す印。
	//
	// 印を必ず残すのは、短いメッセージと「切られた長いメッセージ」を
	// 読み手が区別できるようにするためである。
	elisionSuffix = "…（以下略）"
	// elisionPrefix は先頭を切り落としたことを示す印。
	elisionPrefix = "（前略）…"
)

// truncateTail は s を先頭から最大 n バイトに切り、切った場合は印を付ける。
func truncateTail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return dropPartialRuneAtEnd(s[:n]) + elisionSuffix
}

// truncateHead は s の末尾 n バイトだけを残し、切った場合は印を前に付ける。
//
// 残すのを末尾側にしているのは、コマンドの標準エラー出力では失敗の原因が
// 最後の数行に出るためである（先頭は進捗や警告で埋まる）。
func truncateHead(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return elisionPrefix + dropPartialRuneAtStart(s[len(s)-n:])
}

// dropPartialRuneAtEnd は末尾に残った不完全な UTF-8 の断片を落とす。
// バイト数で切ると多バイト文字の途中で切れ、壊れた文字が JSON に載るためである。
func dropPartialRuneAtEnd(s string) string {
	for s != "" {
		if r, size := utf8.DecodeLastRuneInString(s); r != utf8.RuneError || size > 1 {
			break
		}
		s = s[:len(s)-1]
	}
	return s
}

// dropPartialRuneAtStart は先頭に残った不完全な UTF-8 の断片を落とす。
func dropPartialRuneAtStart(s string) string {
	for s != "" {
		if r, size := utf8.DecodeRuneInString(s); r != utf8.RuneError || size > 1 {
			break
		}
		s = s[1:]
	}
	return s
}

// limitedBuffer は末尾の上限バイトぶんだけを保持する書き込み先。
type limitedBuffer struct {
	buf   []byte
	limit int
}

// Write は末尾の limit バイトを保持し、あふれた古い先頭を捨てる。
//
// 残すのを末尾側にしているのは truncateHead と同じ理由で、失敗の原因は標準エラー
// 出力の最後の数行に出るためである。両者の向きが食い違うと、取り込み段で末尾が
// 落ちた後に truncateHead が末尾を探すことになり、原因の行が画面から消える。
//
// 捨てても成功として返すのは、エラーを返すと os/exec が取り込みを止めてパイプが
// 閉じ、子が書き込みエラーで死んで本来観測したい終了コードが得られなくなるため
// である。上限は「保持量」の制限であって、実行の打ち切りではない。
func (b *limitedBuffer) Write(p []byte) (int, error) {
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
func (b *limitedBuffer) Bytes() []byte { return b.buf }

// String は保持している内容を文字列で返す。
func (b *limitedBuffer) String() string { return string(b.buf) }
