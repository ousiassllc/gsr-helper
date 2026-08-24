package buildconfig

import (
	"bytes"
	"context"
	"encoding/json"
	"maps"
	"os/exec"
	"slices"
	"strings"
	"testing"
	"time"
)

// modulePath は go list が返す import パスの接頭辞。
const modulePath = "github.com/ousiassllc/gsr-helper"

// goListTimeout は go list を打ち切るまでの時間。モジュール全体を 1 回舐めるだけなので
// 通常は数秒で終わる。
const goListTimeout = 120 * time.Second

// graphNodeRule は Go パッケージを mermaid のノードへ畳む規則。prefix はモジュール
// ルートからの相対パスで、**最長プレフィックス一致**した規則の node を採る。
type graphNodeRule struct {
	prefix string
	node   string
}

// graphNodeRules は docs/components/overview.md「## 依存関係」のグラフのノードと
// 実装のパッケージの対応表。
//
// 既存のどのプレフィックスにも当たらない新設パッケージ（新しい最上位のツリー）は、
// ここへ足すまで TestEveryPackageIsMappedToGraphNode が落ちる。
// 一方、既存プレフィックス配下のサブパッケージは最長プレフィックス一致（nodeForPackage）
// で親ノードへ畳まれる。**畳み込みが消すのは同一ノード内部の辺だけである**——サブ
// パッケージでも層をまたいで import すれば、親ノード発の辺として図に要求される
// （Doctor --> Disk を生んでいる唯一の import 元は internal/doctor/hostres である）。
// ここへ規則を足す判断が要るのは、RScope / SetupJob のように独自ノードを与えたいときだけ。
// グラフに描かないと決めた場合は空ノード "" を理由付きで書く。
var graphNodeRules = []graphNodeRule{
	{"cmd/gsr-helper", "Main"},

	// UI 層。ui/template・organism・molecule・atom・token だけが UIParts で、
	// それ以外の internal/ui 以下（親 Model・ui/page 以下・discovery・startup・
	// workscan・ghscope・hostreq・tabset・chrome・keymap）は UIApp へ畳む。
	{"internal/ui/template", "UIParts"},
	{"internal/ui/organism", "UIParts"},
	{"internal/ui/molecule", "UIParts"},
	{"internal/ui/atom", "UIParts"},
	{"internal/ui/token", "UIParts"},
	{"internal/ui", "UIApp"},

	// ドメイン層。
	{"internal/runner/scope", "RScope"},
	{"internal/runner", "Runner"},
	{"internal/svc", "Svc"},
	{"internal/setup/job", "SetupJob"},
	{"internal/setup", "Setup"},
	{"internal/disk", "Disk"},
	{"internal/logs", "Logs"},
	{"internal/doctor", "Doctor"},
	{"internal/config", "Config"},

	// インフラ層。
	{"internal/exec", "Exec"},
	{"internal/gh", "GH"},
	{"internal/audit", "Audit"},
	{"internal/appconfig", "Appconf"},

	// buildconfig はビルド設定とドキュメントの回帰テストだけを置くパッケージで、
	// 本書の層の図に載る実行時の依存ではない。グラフの対象外であることを空ノードで
	// 明示する（表から漏れたのか対象外なのかを区別するため）。
	{"internal/buildconfig", ""},
}

// nodeForPackage はモジュール相対のパッケージパスを mermaid のノード ID へ畳む。
// 対応表に無ければ ok=false を返す。ok=true で空文字ならグラフの対象外。
func nodeForPackage(rel string) (node string, ok bool) {
	best := -1
	for i, r := range graphNodeRules {
		if rel != r.prefix && !strings.HasPrefix(rel, r.prefix+"/") {
			continue
		}
		if best < 0 || len(r.prefix) > len(graphNodeRules[best].prefix) {
			best = i
		}
	}
	if best < 0 {
		return "", false
	}
	return graphNodeRules[best].node, true
}

// uiLayerNodes はノード ID から「そのノードへ畳む規則がすべて internal/ui 配下か」への
// 対応を返す。UI 層の一覧を graphNodeRules から導くことで、図と検査で同じ一覧を
// 2 か所に持たずに済む（UIApp / UIParts は満たし、Runner のような層外は満たさない）。
func uiLayerNodes() map[string]bool {
	inUI := map[string]bool{}
	for _, r := range graphNodeRules {
		if r.node == "" {
			continue // グラフの対象外
		}
		seen, ok := inUI[r.node]
		inUI[r.node] = strings.HasPrefix(r.prefix+"/", "internal/ui/") && (!ok || seen)
	}
	return inUI
}

// uiInternalDrawnEdge は UI 層の内部で唯一グラフに描いてある辺。本書は「粒度の要約である
// 2 ノードの間の 1 本（`UIApp --> UIParts`）だけは例外で、上のグラフに描いてある」と現在形で
// 宣言しており、この 1 本まで UI 層内部として除外すると、散文が宣言する唯一の例外がどの検査
// にも裏取りされず、図から消えても全部緑になる（対になる TestDependencyGraphHasNoStaleEdge は
// 「図に在る辺」しか見ないので、図から消えた辺には反応しない）。ノード名をベタ書きしているのは、
// これが一覧ではなく散文が名指しした 1 本の例外だからである。
var uiInternalDrawnEdge = graphEdge{from: "UIApp", to: "UIParts"}

// relPackage は import パスからモジュールの接頭辞を落とす。モジュール外のパスは
// そのまま返る（呼び手はこれでモジュール内かを判定する）。
func relPackage(importPath string) string {
	return strings.TrimPrefix(importPath, modulePath+"/")
}

// goListPackage は go list -json のうち本検査が使うフィールドだけを持つ。
type goListPackage struct {
	ImportPath string
	Imports    []string
}

// modulePackages は go list -json ./... の結果を返す。
//
// Imports は本番ファイル（GoFiles）の import だけを持ち、テストの import
// （TestImports / XTestImports）は含まない。Issue #151 の言う「直接 import」はこれで
// あり、テスト専用の import が依存グラフに辺を要求しないようにするためこの形にする。
func modulePackages(t *testing.T) []goListPackage {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), goListTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "go", "list", "-json", "./...")
	cmd.Dir = repoRoot(t)
	var out, stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("go list -json ./... に失敗した: %v\n%s", err, stderr.String())
	}

	var pkgs []goListPackage
	dec := json.NewDecoder(&out)
	for dec.More() {
		var p goListPackage
		if err := dec.Decode(&p); err != nil {
			t.Fatalf("go list の JSON を読めない: %v", err)
		}
		pkgs = append(pkgs, p)
	}
	// 対象の取り違え（パスの誤り等）で 0 件になったまま緑になるのを防ぐ。
	if len(pkgs) == 0 {
		t.Fatal("go list がパッケージを 1 件も返さない")
	}
	return pkgs
}

// implEdges は実装の直接 import から生じるノード間の辺と、その根拠の import を返す。
// 同じノードへ畳まれた同士の辺（畳んだノードの内部）は辺として数えない。
//
// go list の Imports は本番ファイルの import なので、テスト専用のフィクスチャ・
// パッケージ（pagetest / cmdtest / setuptest / tabletest）**自身**の import も辺として
// 数える。現在はいずれも本番 import の裏付けがあるが、フィクスチャ限定の import が
// 入ると本番に存在しない辺を図へ描くよう要求することになる。
func implEdges(t *testing.T) map[graphEdge][]string {
	t.Helper()

	edges := map[graphEdge][]string{}
	for _, p := range modulePackages(t) {
		from, ok := nodeForPackage(relPackage(p.ImportPath))
		if !ok || from == "" {
			continue
		}
		for _, imp := range p.Imports {
			rel := relPackage(imp)
			if rel == imp {
				continue // 標準ライブラリと外部依存は層の図に載らない
			}
			to, ok := nodeForPackage(rel)
			if !ok || to == "" || to == from {
				continue
			}
			e := graphEdge{from: from, to: to}
			edges[e] = append(edges[e], p.ImportPath+" -> "+imp)
		}
	}
	// 対象の取り違えで 0 件になったまま緑になるのを防ぐ。
	if len(edges) == 0 {
		t.Fatal("層をまたぐ import から辺を 1 本も作れない" +
			"（modulePath が go.mod の module 宣言とずれていると全 import が外部依存扱いになる）")
	}
	return edges
}

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
