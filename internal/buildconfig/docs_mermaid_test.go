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
)

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

// dependencyMermaid は「## 依存関係」直後の mermaid フェンスの中身を返す。
func dependencyMermaid(t *testing.T) string {
	t.Helper()

	_, tail, ok := strings.Cut(readComponentOverview(t), "## 依存関係")
	if !ok {
		t.Fatalf("%s に「## 依存関係」の節が無い", componentOverviewPath)
	}
	_, tail, ok = strings.Cut(tail, "```mermaid\n")
	if !ok {
		t.Fatalf("%s の「## 依存関係」の後に mermaid フェンスが無い", componentOverviewPath)
	}
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
