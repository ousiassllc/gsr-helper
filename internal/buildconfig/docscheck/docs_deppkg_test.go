package docscheck

// 依存グラフの検査のうち、**実装の側**を読む道具を置く。go list でモジュールの
// パッケージを列挙し、ノード対応表で mermaid のノードへ畳み、層をまたぐ辺を作る
// ところまでを担う。読み取った図と突き合わせる検査そのものは docs_depgraph_test.go に
// あり、図を読む側（フェンスの切り出しと mermaid のパース）は docs_mermaid_test.go に
// ある。1 ファイル 300 行の上限に収めるため、この境界でファイルを分けてある。

import (
	"bytes"
	"context"
	"encoding/json"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/ousiassllc/gsr-helper/internal/buildconfig/buildconfigtest"
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
	cmd.Dir = buildconfigtest.RepoRoot(t)
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
