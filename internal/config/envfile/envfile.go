// Package envfile は runner の .env と .path を読み書きする。
//
// .env はシェルスクリプトではなく KEY=VALUE の羅列として読まれる
// （docs/architecture/data-model.md「.env の扱い」）。編集の差分を最小にするため、
// コメント行・空行・解釈できない行を含めた全行を順序どおり保持し、変更したキーの
// 行だけを置き換える。値の引用符の解釈も変数展開も行わない。行の中身をそのまま
// 持つことが、「変更していない行は 1 バイトも変えない」という約束の土台になる。
package envfile

import (
	"bytes"
	"strings"
)

// line は .env の 1 行。
//
// 改行を text から切り離して eol に持つのは、CRLF のファイルを LF に揃えて
// しまわないためと、末尾に改行が無いファイルをそのまま書き戻すためである
// （その場合は最終行の eol が空文字になる）。
type line struct {
	text string // 改行を含まない行の中身
	eol  string // "\n" / "\r\n" / ""（最終行に改行が無い場合）
	key  string // KEY=VALUE 行なら KEY、それ以外は空
}

// File は .env の中身。全行を順序どおり保持する。
//
// ゼロ値は「空の .env」として使える。
type File struct {
	lines []line
}

// Parse は .env の中身を解析する。解釈できない行も落とさずに保持する。
func Parse(b []byte) File {
	if len(b) == 0 {
		return File{lines: nil}
	}
	s := string(b)
	out := make([]line, 0, strings.Count(s, "\n")+1)
	for len(s) > 0 {
		var text, eol string
		text, eol, s = cut(s)
		out = append(out, line{text: text, eol: eol, key: keyOf(text)})
	}
	return File{lines: out}
}

// cut は s の先頭 1 行を、改行を除いた中身・改行そのもの・残りに分ける。
func cut(s string) (string, string, string) {
	i := strings.IndexByte(s, '\n')
	if i < 0 {
		return s, "", ""
	}
	text, eol := s[:i], "\n"
	if strings.HasSuffix(text, "\r") {
		text, eol = text[:len(text)-1], "\r\n"
	}
	return text, eol, s[i+1:]
}

// keyOf は行が KEY=VALUE 形式なら KEY を返す。コメント行・空行・= を含まない行と、
// = で始まる行（キーが空）は空文字を返す。
//
// 行頭の空白を読み飛ばしてから見るのは、字下げされた行も設定として扱われるため
// である。キーの後ろの空白は落とす（"FOO = 1" のキーは FOO）。
func keyOf(text string) string {
	s := strings.TrimLeft(text, " \t")
	if s == "" || strings.HasPrefix(s, "#") {
		return ""
	}
	i := strings.IndexByte(s, '=')
	if i <= 0 {
		return ""
	}
	return strings.TrimRight(s[:i], " \t")
}

// Bytes は保持している全行を元の並びで書き出す。
// Parse したまま何も変更しなければ、入力と 1 バイトも変わらない。
func (f File) Bytes() []byte {
	var buf bytes.Buffer
	for _, l := range f.lines {
		buf.WriteString(l.text)
		buf.WriteString(l.eol)
	}
	return buf.Bytes()
}

// String は Bytes を文字列で返す。差分プレビュー（config.Diff）へ渡すために使う。
func (f File) String() string { return string(f.Bytes()) }

// Get は key の値を返す。値は trim せず = の後ろをそのまま返す
// （引用符も空白もそのままジョブ環境へ渡るため）。
// 同じキーが複数あれば最初の行の値を返す。
func (f File) Get(key string) (string, bool) {
	i := f.index(key)
	if i < 0 {
		return "", false
	}
	text := f.lines[i].text
	return text[strings.IndexByte(text, '=')+1:], true
}

// Keys は現れる順にキーを返す。同じキーが複数ある場合は最初の 1 つだけを返す。
func (f File) Keys() []string {
	out := make([]string, 0, len(f.lines))
	seen := make(map[string]struct{}, len(f.lines))

	for _, l := range f.lines {
		if l.key == "" {
			continue
		}
		if _, dup := seen[l.key]; dup {
			continue
		}
		seen[l.key] = struct{}{}
		out = append(out, l.key)
	}
	return out
}

// Set は key の行を value で置き換える。他の行（コメント・空行を含む）には触れず、
// 行の位置も字下げも変えない。key の行が無ければ末尾に追加する。
//
// 同じキーが複数ある場合は最初の行だけを書き換える。Get も最初の行を返すため、
// 書いた値がそのまま読み出せる。
func (f *File) Set(key, value string) {
	if i := f.index(key); i >= 0 {
		f.lines[i].text = indentOf(f.lines[i].text) + key + "=" + value
		return
	}
	f.appendLine(key+"="+value, key)
}

// Unset は key の行を取り除く。同じキーが複数あればすべて取り除く。
// 末尾に改行が無いファイルは、その性質を保ったまま取り除く。
func (f *File) Unset(key string) {
	if f.index(key) < 0 {
		return
	}
	last := ""
	if n := len(f.lines); n > 0 {
		last = f.lines[n-1].eol
	}

	out := make([]line, 0, len(f.lines))
	for _, l := range f.lines {
		if l.key != "" && l.key == key {
			continue
		}
		out = append(out, l)
	}
	if n := len(out); n > 0 && last == "" {
		out[n-1].eol = ""
	}
	f.lines = out
}

// index は key の行の位置を返す。無ければ -1。
func (f File) index(key string) int {
	if key == "" {
		return -1
	}
	for i, l := range f.lines {
		if l.key == key {
			return i
		}
	}
	return -1
}

// appendLine は末尾に 1 行足す。直前の行に改行が無ければ足してから追加する。
func (f *File) appendLine(text, key string) {
	eol := f.eol()
	if n := len(f.lines); n > 0 && f.lines[n-1].eol == "" {
		f.lines[n-1].eol = eol
	}
	f.lines = append(f.lines, line{text: text, eol: eol, key: key})
}

// eol はファイルが使っている改行を返す。判別できなければ "\n"。
// 追加する行を既存の行と同じ改行に揃えるために使う。
func (f File) eol() string {
	for _, l := range f.lines {
		if l.eol != "" {
			return l.eol
		}
	}
	return "\n"
}

// indentOf は行頭の空白を返す。元の字下げを保って書き戻すために使う。
func indentOf(text string) string {
	return text[:len(text)-len(strings.TrimLeft(text, " \t"))]
}
