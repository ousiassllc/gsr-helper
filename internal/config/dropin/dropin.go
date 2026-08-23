// Package dropin は systemd の drop-in（override.conf）の生成・解析・読み書きを担う。
//
// 本体のユニットファイルは編集せず drop-in だけを作る
// （docs/architecture/data-model.md「本体ユニットは編集せず drop-in を作る」）。
// 本体を書き換えると、runner の svc.sh が作り直したときに変更が消えるうえ、
// パッケージ更新との衝突も招く。
//
// # .env と違ってコメントと行順を保持しない理由
//
// envfile は「利用者が書いたファイルを本ツールが部分的に直す」関係だが、drop-in は
// 「本ツールが生成して丸ごと置き換えるファイル」であり、引き継ぐべき手書きの文脈が
// 存在しない。保持の仕組みを持つと、[Service] 以外のセクションや解釈できない行も
// 書き戻すことになり、本体ユニットの上書き範囲が本パッケージの意図を超えて広がる。
// 代わりに生成物の先頭へその旨のコメントを入れ、手編集が失われることを利用者にも示す。
package dropin

import "strings"

// header は生成物の先頭に置くコメント。
const header = "# gsr-helper が生成しました。手で書き換えた内容は次の書き込みで失われます。\n"

// section は本パッケージが読み書きする唯一のセクション。
const section = "[Service]"

// Directive は [Service] セクションの 1 行（Key=Value）。
type Directive struct {
	// Key は systemd のディレクティブ名（Restart、MemoryMax など）。
	Key string
	// Value は値。空文字は systemd では「本体の設定を打ち消す」意味を持つ。
	Value string
}

// DropIn は drop-in の中身。[Service] セクションの Directive を順序どおり持つ。
//
// map ではなく列で持つのは、systemd に同じキーを複数回書ける項目
// （Environment= など）があり、map にすると書けなくなるからである。
//
// ゼロ値は「ディレクティブが 1 つも無い drop-in」として使える。
type DropIn struct {
	Directives []Directive
}

// Render は drop-in の INI を組み立てる。
//
// Directive が 1 つも無くても [Service] は書く。セクションの無いファイルを置くと
// systemd が構文エラーとして扱い、ユニット全体の読み込みに失敗するためである。
func (d DropIn) Render() string {
	var b strings.Builder
	b.WriteString(header)
	b.WriteString(section)
	b.WriteString("\n")

	for _, dir := range d.Directives {
		b.WriteString(dir.Key)
		b.WriteString("=")
		b.WriteString(dir.Value)
		b.WriteString("\n")
	}
	return b.String()
}

// Parse は drop-in の INI から [Service] セクションの Directive を順に取り出す。
//
// コメント行（# / ;）・空行・[Service] 以外のセクションは捨てる。捨てたものは
// Render で戻らない（パッケージの doc を参照）。
func Parse(s string) DropIn {
	out := make([]Directive, 0)
	inService := false

	for _, raw := range strings.Split(s, "\n") {
		l := strings.TrimSpace(strings.TrimSuffix(raw, "\r"))
		switch {
		case l == "" || strings.HasPrefix(l, "#") || strings.HasPrefix(l, ";"):
			continue
		case strings.HasPrefix(l, "["):
			inService = strings.EqualFold(l, section)
		case inService:
			if k, v, ok := strings.Cut(l, "="); ok {
				out = append(out, Directive{Key: strings.TrimSpace(k), Value: strings.TrimSpace(v)})
			}
		}
	}
	return DropIn{Directives: out}
}

// Get は key の値を返す。同じキーが複数あれば最初の 1 つ。
//
// 単一値のディレクティブで重複が残っているのは、手で書いた drop-in を読んだ
// 直後だけである（Set が重複を畳む）。
func (d DropIn) Get(key string) (string, bool) {
	if i := d.index(key); i >= 0 {
		return d.Directives[i].Value, true
	}
	return "", false
}

// Set は key の値を差し替える。無ければ末尾に足す。
//
// **同じキーが複数あれば最初の 1 つを書き換えて残りを落とす。** systemd は
// Restart や MemoryMax のような単一値のディレクティブについて**最後に書かれた
// 値**を採るため、最初だけを書き換えて後続を残すと、書き込んだ内容は承認した
// 差分どおりなのに効く値が変わらないという食い違いが起きる。Get も最初の 1 つを
// 返すので、残したままだとフォームの初期値も実際に効いている値とずれる。
func (d *DropIn) Set(key, value string) {
	i := d.index(key)
	if i < 0 {
		d.Directives = append(d.Directives, Directive{Key: key, Value: value})
		return
	}

	d.Directives[i].Value = value

	out := d.Directives[:i+1]
	for _, dir := range d.Directives[i+1:] {
		if dir.Key == key {
			continue
		}
		out = append(out, dir)
	}
	d.Directives = out
}

// Unset は key の行をすべて取り除く。
//
// 「値を空文字にする」（本体の設定を打ち消す）とは意味が違う。打ち消したい
// 場合は Set(key, "") を使う。
func (d *DropIn) Unset(key string) {
	out := make([]Directive, 0, len(d.Directives))
	for _, dir := range d.Directives {
		if dir.Key == key {
			continue
		}
		out = append(out, dir)
	}
	d.Directives = out
}

// index は key の位置を返す。無ければ -1。
func (d DropIn) index(key string) int {
	for i, dir := range d.Directives {
		if dir.Key == key {
			return i
		}
	}
	return -1
}
