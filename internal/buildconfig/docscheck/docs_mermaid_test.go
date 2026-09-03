package docscheck

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/buildconfig/buildconfigtest"
)

// componentOverviewPath は依存グラフと「畳んだノード」の散文の在処。
const componentOverviewPath = "docs/components/overview.md"

// mermaid の各行を判別する。**行全体がその形のもの**だけを拾い、説明文中の矢印や
// ラベルの中身を辺・ノード宣言と取り違えないようにする。
var (
	mermaidEdge     = regexp.MustCompile(`^\s*(\w+)\s*-->\s*(\w+)\s*$`)
	mermaidSubgraph = regexp.MustCompile(`^\s*subgraph\s+(\w+)`)
	mermaidEnd      = regexp.MustCompile(`^\s*end\s*$`)
	mermaidNodeDecl = regexp.MustCompile(`^\s*(\w+)\[`)
	mermaidComment  = regexp.MustCompile(`^\s*%%`)
	// mermaidArrow は mermaid のリンク記法（実線 --/---、点線 -.-、太線 ==、不可視 ~~~、
	// 終端 >/x/o/無し）を行中から広く拾う。mermaidEdge が拾えない形の辺を黙って捨てない
	// ための検出用なので、mermaidEdge 側は厳密なままにしておく。実線を `--` 以上に限るのは
	// cmd/gsr-helper のような 1 個のハイフンを辺と取り違えないため。点線を「ハイフン直後の
	// ドット」で拾うのは、テキスト付き点線リンク `-. text .->` がドットの後もドットであり、
	// ドットの後に `-` を要求すると取りこぼすためである。
	mermaidArrow = regexp.MustCompile(`--+[->xo]|-\.|==+[=>xo]|~~~`)
	// mermaidFenceOpen は mermaid コードフェンスの開始行。CommonMark はフェンスに
	// 3 スペースまでのインデントと 3 個以上のバッククォート／チルダを許し、GitHub も
	// そのすべてを mermaid として描画する（理由は dependencyMermaid を見よ）。
	mermaidFenceOpen = regexp.MustCompile("^ {0,3}(`{3,}|~{3,})mermaid\\b")
)

// soleLine は match に当たる行が text にちょうど 1 つあることを確かめ、その行を返す。
//
// 部分文字列の最初の一致へ無条件にアンカーしない。本書は改訂履歴に本文を逐語引用する
// 慣行があるので解析対象が静かにずれ、また同じ段落が 2 つある状態（先頭が正で 2 つ目の
// 実物が古い、という危険な向き）も見逃す。page/pagetest/doc_test.go の countAtLineStart が
// Issue #96 で塞いだのと同じ欠陥なので、0 個でも 2 個以上でも落とす。
func soleLine(t *testing.T, text, what string, match func(string) bool) string {
	t.Helper()

	var first string
	n := 0
	for _, line := range strings.Split(text, "\n") {
		if !match(line) {
			continue
		}
		if n == 0 {
			first = line
		}
		n++
	}
	switch {
	case n == 0:
		t.Fatalf("%s に%sが無い（書き出しを変えたならこの検査の側も直すこと）", componentOverviewPath, what)
	case n > 1:
		t.Fatalf("%s に%sが %d 個ある（1 つに統合すること）", componentOverviewPath, what, n)
	}
	return first
}

// cutAtLineStart は lead で始まる行を 1 つだけ探し、その直後から末尾までを返す。
// 戻り値には**一致行の残り**（lead の直後から行末まで）が含まれる——呼び手はその行の
// 続きを読む（collapsedNodeProse が `UIParts` は 以降を必要としている）。
// 行頭に 1 個であることを要求する理由は soleLine を見よ。
func cutAtLineStart(t *testing.T, text, lead string) string {
	t.Helper()

	soleLine(t, text, fmt.Sprintf("%q で始まる行", lead), func(line string) bool {
		return strings.HasPrefix(line, lead)
	})
	_, tail, _ := strings.Cut("\n"+text, "\n"+lead)
	return tail
}

// graphEdge は mermaid の 1 本の辺。
type graphEdge struct{ from, to string }

// depGraph は docs/components/overview.md「## 依存関係」の mermaid グラフ。
// subgraph は subgraph の ID から、そこに宣言されたノード ID の集合への対応。
type depGraph struct {
	edges    map[graphEdge]bool
	subgraph map[string]map[string]bool
}

// readComponentOverview はコンポーネント設計の本文を返す。
func readComponentOverview(t *testing.T) string {
	t.Helper()

	body, err := os.ReadFile(filepath.Join(buildconfigtest.RepoRoot(t), filepath.FromSlash(componentOverviewPath)))
	if err != nil {
		t.Fatalf("%s を読めない: %v", componentOverviewPath, err)
	}
	return string(body)
}

// dependencyMermaid は行頭の見出し「## 依存関係」の直後の mermaid フェンスの中身を返す。
//
// フェンスの開始行も soleLine で**節の中にちょうど 1 個**と数える。最初の一致へ無条件に
// アンカーすると、本文を逐語引用する改訂履歴の慣行でグラフの改訂前/改訂後が併記された
// とき解析対象が静かにずれ、レンダリング後に読者が見る 2 つ目のグラフが無検査になる
// （soleLine の doc コメントが見出し・段落について述べているのと同じ理由がフェンスにも
// 当てはまる）。
//
// 数える範囲は「## 依存関係」節の中に限る。文書末尾まで数えると、後方の無関係な節が
// 図（シーケンス図など）を持つだけで「1 つに統合すること」という**直す先の分からない**
// 助言で落ちるためである。
//
// 開始行の判定に mermaidFenceOpen を使うのは、`strings.HasPrefix(line, "```mermaid")` が
// CommonMark の許す形（3 スペースまでのインデント、4 個以上のバッククォート、チルダ）を
// 数え落とすからである。とりわけ 4 個以上のバッククォートは**フェンスを逐語引用する**
// 標準的な書き方で、本書の「改訂履歴に本文を逐語引用する」慣行で最も起きやすい形である。
// GitHub はいずれも mermaid として描画するので、読者が見る 2 つ目のグラフを検査が
// 見落とすことになる。見出し・段落の側（cutAtLineStart）の厳密さは保つ。
func dependencyMermaid(t *testing.T) string {
	t.Helper()

	tail := cutAtLineStart(t, readComponentOverview(t), "## 依存関係")
	if i := strings.Index(tail, "\n## "); i >= 0 {
		tail = tail[:i] // 次の節より前だけを見る
	}

	open := soleLine(t, tail, "「## 依存関係」節の mermaid フェンスの開始行", mermaidFenceOpen.MatchString)
	// 閉じフェンスは開始行と**同じ文字・開始以上の個数**でなければならない（CommonMark）。
	fence := mermaidFenceOpen.FindStringSubmatch(open)[1]
	closer := regexp.MustCompile(fmt.Sprintf("^ {0,3}%s{%d,}[ \t]*$", regexp.QuoteMeta(fence[:1]), len(fence)))

	lines := strings.Split(tail, "\n")
	start := slices.Index(lines, open)
	for i := start + 1; i < len(lines); i++ {
		if closer.MatchString(lines[i]) {
			return strings.Join(lines[start+1:i], "\n")
		}
	}
	t.Fatalf("%s の mermaid フェンスが閉じていない", componentOverviewPath)
	return ""
}

// parseDepGraph は mermaid の辺と subgraph を読み取る。
//
// subgraph は実物ではネストしていないが、end で閉じるスタックとして扱っておけば
// ネストしても壊れない。
func parseDepGraph(t *testing.T) depGraph {
	t.Helper()

	g := depGraph{edges: map[graphEdge]bool{}, subgraph: map[string]map[string]bool{}}
	var open []string
	for _, line := range strings.Split(dependencyMermaid(t), "\n") {
		switch {
		case mermaidComment.MatchString(line):
			// mermaid のコメント行。矢印を書いても辺ではないので矢印ガードより先に飛ばす。
		case mermaidSubgraph.MatchString(line):
			id := mermaidSubgraph.FindStringSubmatch(line)[1]
			if g.subgraph[id] == nil {
				g.subgraph[id] = map[string]bool{}
			}
			open = append(open, id)
		case mermaidEnd.MatchString(line):
			if len(open) == 0 {
				t.Fatalf("mermaid の end が subgraph より多い: %q", line)
			}
			open = open[:len(open)-1]
		case mermaidEdge.MatchString(line):
			m := mermaidEdge.FindStringSubmatch(line)
			g.edges[graphEdge{from: m[1], to: m[2]}] = true
		case mermaidArrow.MatchString(line):
			// 矢印なのにここまで来た＝この形の辺に mermaidEdge が対応していない。
			// 黙って捨てると図にある辺を「グラフに無い」と実態と逆の理由で落とす。
			// **ノード宣言より先に見る。** `UIApp[ui] --> Logs` のように矢印の左辺で
			// ノードを宣言する記法が mermaidNodeDecl に消費されると、辺が捨てられた
			// うえこのガードにも届かない。
			t.Fatalf("mermaid のこの形の辺に対応していない（mermaidEdge が拾えない）: %q", line)
		case mermaidNodeDecl.MatchString(line):
			id := mermaidNodeDecl.FindStringSubmatch(line)[1]
			for _, sg := range open {
				g.subgraph[sg][id] = true
			}
		}
	}
	if len(open) != 0 {
		t.Fatalf("mermaid の subgraph %v が end で閉じていない", open)
	}
	// フェンスや行の形の取り違えで 0 本になったまま緑になるのを防ぐ。
	if len(g.edges) == 0 {
		t.Fatal("mermaid グラフから辺を 1 本も読み取れない")
	}
	return g
}
