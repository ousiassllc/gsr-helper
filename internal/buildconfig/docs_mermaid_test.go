package buildconfig

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
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
)

// cutAtLineStart は lead で始まる行を 1 つだけ探し、その直後から末尾までを返す。
//
// 部分文字列の最初の一致へ無条件にアンカーしない。本書は改訂履歴に本文を逐語引用する
// 慣行があるので解析対象が静かにずれ、また同じ段落が 2 つある状態（先頭が正で 2 つ目の
// 実物が古い、という危険な向き）も見逃す。page/pagetest/doc_test.go の countAtLineStart が
// Issue #96 で塞いだのと同じ欠陥なので、0 個でも 2 個以上でも落とす。
func cutAtLineStart(t *testing.T, text, lead string) string {
	t.Helper()

	n := 0
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, lead) {
			n++
		}
	}
	switch {
	case n == 0:
		t.Fatalf("%s に %q で始まる行が無い（書き出しを変えたならこの検査の側も直すこと）",
			componentOverviewPath, lead)
	case n > 1:
		t.Fatalf("%s に %q で始まる行が %d 個ある（1 つに統合すること）", componentOverviewPath, lead, n)
	}
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

	body, err := os.ReadFile(filepath.Join(repoRoot(t), filepath.FromSlash(componentOverviewPath)))
	if err != nil {
		t.Fatalf("%s を読めない: %v", componentOverviewPath, err)
	}
	return string(body)
}

// dependencyMermaid は行頭の見出し「## 依存関係」の直後の mermaid フェンスの中身を返す。
//
// フェンスの開始行も cutAtLineStart で**行頭に 1 個**と数える。最初の一致へ無条件に
// アンカーすると、本文を逐語引用する改訂履歴の慣行でグラフの改訂前/改訂後が併記された
// とき解析対象が静かにずれ、レンダリング後に読者が見る 2 つ目のグラフが無検査になる
// （cutAtLineStart の doc コメントが見出し・段落について述べているのと同じ理由が
// フェンスにも当てはまる）。見出し以降の tail 全体（文書末尾の改訂履歴まで）で数える
// ことになるが、本書の mermaid フェンスは現在この 1 個だけである。
func dependencyMermaid(t *testing.T) string {
	t.Helper()

	tail := cutAtLineStart(t, readComponentOverview(t), "## 依存関係")
	tail = cutAtLineStart(t, tail, "```mermaid")
	body, _, ok := strings.Cut(tail, "\n```")
	if !ok {
		t.Fatalf("%s の mermaid フェンスが閉じていない", componentOverviewPath)
	}
	return body
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
