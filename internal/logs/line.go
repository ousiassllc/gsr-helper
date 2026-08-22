package logs

import "strings"

// ログ 1 行の表現と、強調表示（FR-25）に使う重大度の判定を置く。

// Level はログ 1 行の重大度。
//
// 色ではなく重大度を返すのは、表示層でない層が色を決めないためである
// （atomic-design.md の依存の規則。token.Styles を持つのは UI だけ）。
type Level int

// Level の取り得る値。
const (
	// LevelPlain は強調しない行。
	LevelPlain Level = iota
	// LevelWarn は WARN を含む行。
	LevelWarn
	// LevelError は ERROR を含む行。
	LevelError
)

// Line はログ 1 行。
//
// 重大度を行に持たせるのは、判定を送出時の 1 回に限るためである。描画のたびに
// 判定すると、追従中の画面（毎秒書き換わる）で行数ぶんの走査が繰り返される。
type Line struct {
	Text  string
	Level Level
}

// NewLine は本文から重大度を判定した 1 行を作る。
func NewLine(text string) Line {
	return Line{Text: text, Level: Classify(text)}
}

// Classify は行の重大度を判定する（FR-25 の `ERROR` / `WARN` の強調表示）。
//
// runner のログは `[2026-08-21 12:05:44Z ERROR JobRunner] ...` のように重大度を
// 角括弧の中へ埋める。`journalctl` の行も同じ語を含むので判定を分けない。
//
// **語として現れる場合だけを見る。** 部分一致にすると `ERRORLEVEL` や
// `no-warnings` を含む行が赤くなり、実際の異常が埋もれる。大文字の語だけを
// 対象にするのは runner のログが重大度を大文字で書くためで、本文中の英単語
// （`error occurred`）まで拾うと強調が意味を失う。
//
// ERROR を先に見るのは、1 行に両方が現れた場合に重い方を採るためである。
func Classify(text string) Level {
	switch {
	case hasWord(text, "ERROR"):
		return LevelError
	case hasWord(text, "WARN"):
		return LevelWarn
	default:
		return LevelPlain
	}
}

// hasWord は word が英数字に挟まれない形で現れるかを返す。
//
// 単語の境界を「英字・数字・アンダースコアでないこと」で見る。ログの区切りは
// 空白・角括弧・コロンのいずれもあり得るため、区切り文字を列挙するより
// 「続きがない」ことを見るほうが取りこぼさない。
func hasWord(text, word string) bool {
	for i := 0; ; {
		j := strings.Index(text[i:], word)
		if j < 0 {
			return false
		}
		start := i + j
		end := start + len(word)
		if !isWordByte(text, start-1) && !isWordByte(text, end) {
			return true
		}
		i = start + 1
	}
}

// isWordByte は i 番目が語を構成する文字かを返す。範囲外は偽（境界）とする。
func isWordByte(s string, i int) bool {
	if i < 0 || i >= len(s) {
		return false
	}
	c := s[i]
	switch {
	case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '_':
		return true
	default:
		return false
	}
}
