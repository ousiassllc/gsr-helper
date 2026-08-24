package docscheck

// 読み取った依存グラフ（docs_mermaid_test.go）と実装の辺（docs_deppkg_test.go）を
// 突き合わせる検査を置く。図と実装のどちらかを読む道具はここには置かない。

import (
	"maps"
	"slices"
	"strings"
	"testing"
)

// uiInternalDrawnEdge は UI 層の内部で唯一グラフに描いてある辺。本書は「粒度の要約である
// 2 ノードの間の 1 本（`UIApp --> UIParts`）だけは例外で、上のグラフに描いてある」と現在形で
// 宣言しており、この 1 本まで UI 層内部として除外すると、散文が宣言する唯一の例外がどの検査
// にも裏取りされず、図から消えても全部緑になる（対になる TestDependencyGraphHasNoStaleEdge は
// 「図に在る辺」しか見ないので、図から消えた辺には反応しない）。ノード名をベタ書きしているのは、
// これが一覧ではなく散文が名指しした 1 本の例外だからである。
var uiInternalDrawnEdge = graphEdge{from: "UIApp", to: "UIParts"}

// sortedEdges は辺を安定した順序で返す。map の反復順のままだと、同じ食い違いでも
// 実行のたびにエラーの並びが変わって差分を追いにくい。
func sortedEdges[V any](m map[graphEdge]V) []graphEdge {
	keys := make([]graphEdge, 0, len(m))
	for e := range m {
		keys = append(keys, e)
	}
	slices.SortFunc(keys, func(a, b graphEdge) int {
		if c := strings.Compare(a.from, b.from); c != 0 {
			return c
		}
		return strings.Compare(a.to, b.to)
	})
	return keys
}

// モジュールの全パッケージが、依存グラフのノード対応表のどれかに一致しなければならない。
//
// この検査が無いと、新設したパッケージが対応表から漏れたまま「辺が足りない」検査の
// 網からも静かに外れ、以降どれだけ依存を増やしてもグラフとの食い違いを検知できなくなる
// （Issue #151。同種の辺の欠落は改訂 1.22 / 1.27 / 1.29 / 1.32 / 1.33 / 1.40 と
// 6 度再発している）。
func TestEveryPackageIsMappedToGraphNode(t *testing.T) {
	for _, p := range modulePackages(t) {
		if _, ok := nodeForPackage(relPackage(p.ImportPath)); !ok {
			t.Errorf("新しいパッケージ %s がノード対応表に無い"+
				"（どのノードへ畳むか決めて graphNodeRules へ足すこと。"+
				"グラフに描かない場合は空ノードで明示すること）", p.ImportPath)
		}
	}
}

// 実装にある層をまたぐ直接 import は、すべて依存グラフに辺として描かれていなければ
// ならない。
//
// 辺の欠落は「その依存は存在しない」と読まれ、正当な import が規則違反と判定される。
// 同種の欠落は改訂 1.22 / 1.27 / 1.29 / 1.32 / 1.33 / 1.40 で 6 度再発しており、
// 機械的に検知するためこの検査を置く（Issue #151）。
//
// UI 層のノード同士の辺だけは対象外にする。本書は「**本書が描かないのは UI パッケージ
// 同士の依存である**——ただし粒度の要約である 2 ノードの間の 1 本（`UIApp --> UIParts`）
// だけは例外」と宣言しており、`organism` → `keymap` のような辺は
// docs/ui/atomic-design.md の依存グラフが持つ。どのノードが UI 層かは mermaid の
// subgraph UI から読み取る（ここにノード名をベタ書きすると図と二重管理になる）。
// ただし散文が例外として名指しする 1 本（uiInternalDrawnEdge）だけは除外しない。除外すると
// 「上のグラフに描いてある」という宣言がどの検査にも裏取りされず、図から消せてしまう。
func TestDependencyGraphDrawsEveryCrossLayerImport(t *testing.T) {
	g := parseDepGraph(t)

	ui := g.subgraph["UI"]
	// UI 層を取り違えるとすべての辺が除外されて検査が空振りする。
	if len(ui) == 0 {
		t.Fatal("mermaid グラフに subgraph UI のノードが無い（UI 層内部の辺を除外できない）")
	}
	// 名指しした 1 本の**両端が今も図に在る**ことを表明する。図と graphNodeRules の
	// 両方でノードを改名すると（UIApp → UIMain など）下の除外条件 e != uiInternalDrawnEdge が
	// 恒真になり、UI 層内部の一律除外がこの 1 本を再び飲み込む。対の
	// TestDependencyGraphHasNoStaleEdge は「図に在る辺」しか見ないので反応しない。
	if !ui[uiInternalDrawnEdge.from] || !ui[uiInternalDrawnEdge.to] {
		t.Fatalf("散文が例外として名指しする %s --> %s が subgraph UI に無い"+
			"（ノードを改名したなら uiInternalDrawnEdge も直すこと）",
			uiInternalDrawnEdge.from, uiInternalDrawnEdge.to)
	}
	// 散文は「例外は 1 本だけ」と現在形で宣言している。2 本目を描いても、その辺には
	// UI 層内部の実装 import（organism → keymap 等）が裏付けとして必ず在るので
	// TestDependencyGraphHasNoStaleEdge は通ってしまう。本数をここで表明して
	// 散文・図・検査の 3 者をそろえる。
	for _, e := range sortedEdges(g.edges) { // 報告の順序を決定的にする
		if ui[e.from] && ui[e.to] && e != uiInternalDrawnEdge {
			t.Errorf("UI 層内部の辺 %s --> %s が図にある"+
				"（本書が描く UI 内部の辺は %s --> %s の 1 本だけである。"+
				"内部の辺は docs/ui/atomic-design.md の依存グラフが持つ）",
				e.from, e.to, uiInternalDrawnEdge.from, uiInternalDrawnEdge.to)
		}
	}
	// 全損だけでなく**部分的な広がり**も見る。subgraph UI にノードを 1 つ移すだけで
	// そのノードとの辺がまとめて除外され、層をまたぐ辺が無検査になるため。
	inUI := uiLayerNodes()
	for _, n := range slices.Sorted(maps.Keys(ui)) { // 報告の順序を決定的にする
		if !inUI[n] {
			t.Fatalf("subgraph UI に UI 層でないノード %s がある"+
				"（UI 層内部の辺の除外範囲が黙って広がり、層をまたぐ辺が無検査になる）", n)
		}
	}

	edges := implEdges(t)
	for _, e := range sortedEdges(edges) {
		if g.edges[e] || (ui[e.from] && ui[e.to] && e != uiInternalDrawnEdge) {
			continue
		}
		reasons := edges[e]
		if len(reasons) > 3 {
			reasons = reasons[:3]
		}
		t.Errorf("実装にある %s --> %s の辺が依存グラフに無い"+
			"（辺の欠落は「その依存は存在しない」と読まれる）\nこの辺を生んでいる import:\n  %s",
			e.from, e.to, strings.Join(reasons, "\n  "))
	}
}

// 依存グラフの辺は、すべて実装の直接 import に裏付けられていなければならない。
//
// 実装から消えた依存が図に残ると、存在しない依存を前提に設計を読むことになる。
// 辺の欠落（Issue #151）と対になる逆向きの検査であり、両方向を見て初めて図と実装の
// 一致を保証できる。
func TestDependencyGraphHasNoStaleEdge(t *testing.T) {
	g := parseDepGraph(t)
	edges := implEdges(t)

	for _, e := range sortedEdges(g.edges) {
		if len(edges[e]) == 0 {
			t.Errorf("依存グラフの %s --> %s に対応する import が実装に 1 本も無い"+
				"（実装から消えた依存が図に残っている）", e.from, e.to)
		}
	}
}
