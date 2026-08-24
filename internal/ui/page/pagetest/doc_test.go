package pagetest_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// atomic-design.md の共有部品の列挙が import_test.go の shared と一致することを検査する。
//
// **写しは 3 箇所ある。** どれが共有部品でどれがタブかの正は shared だが、読み手の
// 入口が 3 つあるため文書側にも 3 つの写しが置いてある——「`page/` は 1 ディレクトリ
// 1 タブではない」の段落（決め方を読む場所）・ディレクトリ構成のツリー（置き場所を
// 探す場所）・実装状況の「実装済み」の行（部品が有るかを引く場所）。段落だけを検査
// していた頃、残る 2 つは古くなっても緑のままだった（Issue #130）。
//
// 依存の向きの検査（import_test.go）と分けてあるのは、見ている対象が実装の import と
// 文書の散文で違うためである。ファイルを分けた直接の理由は 1 ファイル 300 行の上限だが、
// 境界はこの責務の違いに沿わせた（docs/ui/atomic-design.md「行数の予算」の「ファイルの行数」）。

// docShared は atomic-design.md の散文から、単体でバッククォートに囲まれた
// `page/<名前>` を拾う。`page/pagetest/import_test.go` のようにパスが続くものや、
// `page/<名前>` というプレースホルダは閉じバッククォートが直後に来ないため拾わない。
//
// 文字クラスを [a-z][a-z0-9_]* にしてあるのは、数字やアンダースコアを含む
// ディレクトリ名（Go のパッケージ名として妥当である）を将来足したときに、
// **検査が黙って対象から外れる**のを防ぐためである。[a-z]+ だと `page/modal2` は
// 1 文字も拾われず、列挙が食い違っていても緑になる。
var docShared = regexp.MustCompile("`page/([a-z][a-z0-9_]*)`")

// docTreeEntry はディレクトリ構成のツリーから page/ 直下のエントリを拾う。
//
// 末尾の `/` の直後に空白を要求するので、ネストしたもの（`page/runners/rowview/`）
// と `page/` 自身は拾わない。プレースホルダの `page/<tab>/` も文字クラスに `<` が
// 無いため拾わない。
var docTreeEntry = regexp.MustCompile(`(?m)^\s*page/([a-z][a-z0-9_]*)/\s`)

// docSharedLead は当該段落の書き出し。段落の同定と、重複の検出に使う。
//
// **行頭に来るものだけを数える。** 素朴に strings.Count で全体を数えると、他の節が
// この段落を引用しただけで「段落が 2 つある」と誤検出する。文書側にもこの制約を
// 明記してある（atomic-design.md のこの段落の直後）。
const docSharedLead = "**`page/` は「1 ディレクトリ 1 タブ」ではない。**"

// docTreeLead はディレクトリ構成のツリーを含む節の見出し。
const docTreeLead = "## ディレクトリ構成"

// docImplLead は実装状況の表の「実装済み」の行の書き出し。
const docImplLead = "| 実装済み |"

// atomicDesignPath は atomic-design.md の絶対パスを返す。
func atomicDesignPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(moduleRoot(t), "docs", "ui", "atomic-design.md")
}

// readAtomicDesign は atomic-design.md を読む。
func readAtomicDesign(t *testing.T) string {
	t.Helper()

	path := atomicDesignPath(t)
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%s を読めない: %v", path, err)
	}
	return string(body)
}

// countAtLineStart は sub が行頭に現れる回数を返す。
func countAtLineStart(text, sub string) int {
	n := 0
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, sub) {
			n++
		}
	}
	return n
}

// lineStartingWith は prefix で始まる最初の行を丸ごと返す。
//
// strings.Cut を全文へ当てないのは、`| 実装済み |` のように**表のセルとしても
// 現れる**書き出しがあるためである。全文の最初の一致を採ると、行頭ではない
// セルを拾って以降の検査が空振りする。
func lineStartingWith(text, prefix string) (string, bool) {
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, prefix) {
			return line, true
		}
	}
	return "", false
}

// pageDirs は internal/ui/page 直下に実在するディレクトリ名を返す。
func pageDirs(t *testing.T) map[string]bool {
	t.Helper()

	dir := filepath.Join(moduleRoot(t), "internal", "ui", "page")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("%s を読めない: %v", dir, err)
	}

	out := map[string]bool{}
	for _, e := range entries {
		if e.IsDir() {
			out[e.Name()] = true
		}
	}
	if len(out) == 0 {
		t.Fatal("page/ 直下のディレクトリを 1 つも見つけられなかった（検査が空振りしている）")
	}
	return out
}

// atomic-design.md の共有部品の列挙は shared と一致する。
//
// 本書は「どれがタブでどれが共有部品かは shared が決める」と定めながら、同じ説明の
// 段落を 2 つ持ち、しかも列挙が食い違っていた（片方が runnerop を欠き、両方が
// progressmodal を欠いていた）。読み手はどちらを写しても shared とずれる（Issue #96）。
//
// 正は shared の側なので、この検査は**文書が shared に追従しているか**だけを見る。
func TestSharedPackagesMatchDoc(t *testing.T) {
	text := readAtomicDesign(t)

	// **行頭に来るものだけを数える。** 他の節がこの書き出しを引用しただけで
	// 「段落が 2 つある」と誤検出しないため（この制約は文書側にも書いてある）。
	switch n := countAtLineStart(text, docSharedLead); {
	case n == 0:
		t.Fatalf("atomic-design.md に %q で始まる段落が無い（行頭に 1 つ置くこと。"+
			"見出しや書き出しを変えたならこの検査の docSharedLead も直すこと）", docSharedLead)
	case n > 1:
		t.Fatalf("atomic-design.md に %q で始まる段落が行頭に %d 個ある（1 つに統合すること）", docSharedLead, n)
	}

	_, tail, _ := strings.Cut(text, docSharedLead)
	para, _, _ := strings.Cut(tail, "\n\n")

	listed := map[string]bool{}
	for _, m := range docShared.FindAllStringSubmatch(para, -1) {
		listed[m[1]] = true
	}
	for name := range shared {
		if !listed[name] {
			t.Errorf("atomic-design.md の列挙に page/%s が無い（shared には載っている）", name)
		}
	}
	for name := range listed {
		if !shared[name] {
			t.Errorf("atomic-design.md が page/%s を共有部品として挙げているが shared に無い", name)
		}
	}
}

// ディレクトリ構成のツリーの page/ 直下の並びは shared と一致する。
//
// **同じ事実を述べる列挙は 1 つではない。** 「1 ディレクトリ 1 タブではない」の段落
// だけを検査していた頃、ツリーはその検査の外にあった（Issue #130）。ツリーは
// 「新しい部品をどこへ置くか」を最初に引く場所なので、ここが古いと同じ責務の
// パッケージがもう 1 つ作られる。
//
// タブは `page/<tab>/` の 1 行にまとめてあるため、ツリーに個別の名前で現れる
// `page/<名前>/` は共有部品だけである。したがって両方向で突き合わせられる。
func TestDirectoryTreeMatchesShared(t *testing.T) {
	text := readAtomicDesign(t)

	if n := countAtLineStart(text, docTreeLead); n != 1 {
		t.Fatalf("atomic-design.md の %q が行頭に %d 個ある（1 つであること）", docTreeLead, n)
	}
	_, tail, _ := strings.Cut(text, docTreeLead)
	_, body, ok := strings.Cut(tail, "```\n") // 開きフェンスを落とす
	if !ok {
		t.Fatal("ディレクトリ構成のツリーの開きフェンスを見つけられない")
	}
	fence, _, ok := strings.Cut(body, "\n```")
	if !ok {
		t.Fatal("ディレクトリ構成のツリーの閉じフェンスを見つけられない")
	}

	listed := map[string]bool{}
	for _, m := range docTreeEntry.FindAllStringSubmatch(fence, -1) {
		listed[m[1]] = true
	}
	if len(listed) == 0 {
		t.Fatal("ツリーから page/ 直下のエントリを 1 つも拾えなかった（検査が空振りしている）")
	}

	for name := range shared {
		if !listed[name] {
			t.Errorf("ディレクトリ構成のツリーに page/%s/ が無い（shared には載っている）", name)
		}
	}
	for name := range listed {
		if !shared[name] {
			t.Errorf("ディレクトリ構成のツリーが page/%s/ を個別に挙げているが shared に無い"+
				"（タブは page/<tab>/ の 1 行にまとめること）", name)
		}
	}
}

// 実装状況の「実装済み」の行は、共有部品を 1 つも落とさない。
//
// この行は「その部品が有るか」を後続 Issue が最初に引く場所である（Issue #130）。
// 共有部品が載っていないと、無いものとして同じ責務がもう 1 つ作られる。
//
// **片方向にしか突き合わせない。** この行はタブも列挙するので shared と一致は
// しない。代わりに、挙げてある page/<名前> が実在することを page/ 直下の
// ディレクトリで確かめる（移動・改名で消えた名前が残らないようにする）。
func TestImplementedListCoversSharedPackages(t *testing.T) {
	text := readAtomicDesign(t)

	if n := countAtLineStart(text, docImplLead); n != 1 {
		t.Fatalf("実装状況の %q で始まる行が %d 個ある（1 つであること）", docImplLead, n)
	}
	row, ok := lineStartingWith(text, docImplLead)
	if !ok {
		t.Fatalf("実装状況の %q で始まる行が無い", docImplLead)
	}

	listed := map[string]bool{}
	for _, m := range docShared.FindAllStringSubmatch(row, -1) {
		listed[m[1]] = true
	}
	if len(listed) == 0 {
		t.Fatal("実装済みの行から page/<名前> を 1 つも拾えなかった（検査が空振りしている）")
	}

	for name := range shared {
		if !listed[name] {
			t.Errorf("実装状況の「実装済み」に page/%s が無い（shared には載っている）", name)
		}
	}
	dirs := pageDirs(t)
	for name := range listed {
		if !dirs[name] {
			t.Errorf("実装状況の「実装済み」が page/%s を挙げているが internal/ui/page/%s が無い", name, name)
		}
	}
}
