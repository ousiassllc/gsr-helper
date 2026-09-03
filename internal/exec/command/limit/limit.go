// Package limit は文字列を上限バイト数へ収める道具を置く。切り詰めて印を残す
// Tail / Head と、末尾の上限ぶんだけを保持する Buffer の 2 組である。
//
// 親ディレクトリの command（コマンドの実行・監査・タイムアウト）から切り出して
// あるのは、**増え方が違い、依存の向きを強制できる**ためである——command の側は
// 実行の仕組み（プロセスグループ・タイムアウト・監査レコードの欄）が増えれば
// 伸びるが、こちらが増えるのは**どちらの端を残し、断片をどう落とし、印をどう
// 付けるか**が変わったときだけである。このパッケージは command を一切 import せず、
// Command / ExitError / 監査レコードを知らない。渡されるのは文字列とバイト数
// だけなので、「監査に載せるときだけ印を省く」ような迂回を書けない。
//
// **上限の値そのものはここに持たない。** どこに何バイトの上限を置くかは呼び出し側
// の方針であり、command の limits.go が持つ（判断は docs/ui/atomic-design.md の
// 「`internal/exec/command` から切り詰めの道具を `limit` へ切り出した判断」）。
package limit

import "unicode/utf8"

const (
	// Suffix は末尾を切り落としたことを示す印。
	//
	// 印を必ず残すのは、短いメッセージと「切られた長いメッセージ」を
	// 読み手が区別できるようにするためである。
	Suffix = "…（以下略）"
	// Prefix は先頭を切り落としたことを示す印。
	Prefix = "（前略）…"
)

// Tail は s を先頭から最大 n バイトに切り、切った場合は印を付ける。
func Tail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return dropPartialRuneAtEnd(s[:n]) + Suffix
}

// Head は s の末尾 n バイトだけを残し、切った場合は印を前に付ける。
//
// 残すのを末尾側にしているのは、コマンドの標準エラー出力では失敗の原因が
// 最後の数行に出るためである（先頭は進捗や警告で埋まる）。
func Head(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return Prefix + dropPartialRuneAtStart(s[len(s)-n:])
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
// 落とさないと Head が返す文字列は Prefix の直後が壊れた状態になる。
func dropPartialRuneAtStart(s string) string {
	for s != "" {
		if r, size := utf8.DecodeRuneInString(s); r != utf8.RuneError || size > 1 {
			break
		}
		s = s[1:]
	}
	return s
}
