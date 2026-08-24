package docscheck

// 依存グラフの検査のうち、**畳んだノードの内訳**を見る側を置く。overview.md の
// 「畳んだノード」の段落が挙げる UIApp / UIParts の内訳と、internal/ui 直下の実装の
// ツリーを突き合わせる。図の辺を読むのは docs_mermaid_test.go、実装の import を読むのは
// docs_deppkg_test.go、その両者の辺を突き合わせるのは docs_depgraph_test.go であり、
// ここは辺を 1 本も見ない——見るのは「どのパッケージがどちらのノードへ畳まれるか」だけである。

import (
	"regexp"
	"slices"
	"strings"
	"testing"
)

// collapsedNodeLead は「畳んだノード」を宣言する段落の先頭。
const collapsedNodeLead = "**UI 層の 2 ノードは粒度の要約である。**"

// collapsedNodeName は散文の列挙から畳んだ UI パッケージの名前を拾う。
//
// 散文は `ui/template` / `organism` のように接頭辞の有無が混在するので、
// バッククォートに囲まれた「`ui/<名前>` または 1 語の小文字英字」だけを対象にする。
// この形にすることで `UIApp`（大文字を含む）や `internal/ui`（`internal/` で始まる）を
// 誤って拾わない。
//
// 文字クラスを [a-z][a-z0-9_]* にしてあるのは、数字やアンダースコアを含む
// ディレクトリ名（Go のパッケージ名として妥当である）を将来足したときに、**散文の側だけ
// 検査から外れる**のを防ぐためである（page/pagetest/doc_test.go の docShared が同じ理由で
// 同じ文字クラスを使っている）。
var collapsedNodeName = regexp.MustCompile("`(?:ui/)?([a-z][a-z0-9_]*)`")

// 「畳んだノード」の散文の列挙が、実装の internal/ui 直下のパッケージと一致しなければ
// ならない。
//
// UIApp / UIParts は複数のパッケージを 1 ノードに要約したものなので、どのパッケージが
// どちらに畳まれているかは散文にしか書かれていない。ここが古いと、新設パッケージが
// どのノードの辺で代表されるのかを読み手が確かめられず、グラフの辺の欠落
// （改訂 1.22 / 1.27 / 1.29 / 1.32 / 1.33 / 1.40 で 6 度再発）を人の目でも
// 見つけられなくなる（Issue #151）。
func TestCollapsedUINodesMatchDoc(t *testing.T) {
	appProse, partsProse := collapsedNodeProse(t)
	appImpl, partsImpl := collapsedNodeImpl(t)

	compareCollapsedNames(t, "UIApp", appImpl, appProse)
	compareCollapsedNames(t, "UIParts", partsImpl, partsProse)
}

// collapsedNodeProse は散文が挙げている UIApp / UIParts の畳んだパッケージ名を返す。
// 段落は行頭に 1 つでなければならない（理由は cutAtLineStart を見よ）。
func collapsedNodeProse(t *testing.T) (app, parts []string) {
	t.Helper()

	tail := cutAtLineStart(t, readComponentOverview(t), collapsedNodeLead)
	appHalf, partsHalf, ok := strings.Cut(tail, "`UIParts` は ")
	if !ok {
		t.Fatalf("%s の段落を `UIParts` の宣言で 2 つに割れない", componentOverviewPath)
	}
	return collapsedNames(t, "UIApp", cutEnumeration(t, appHalf, "を畳んだものであり")),
		collapsedNames(t, "UIParts", cutEnumeration(t, partsHalf, "を束ねたものである"))
}

// cutEnumeration は列挙が終わる語で末尾を切る。切らないと、同じ段落の後続の文
// （`ui/tabset` → `runner` など）に出てくる名前まで列挙として拾ってしまう。
func cutEnumeration(t *testing.T, half, end string) string {
	t.Helper()

	body, _, ok := strings.Cut(half, end)
	if !ok {
		t.Fatalf("%s の散文に「%s」が無い（列挙の終わりを特定できない）", componentOverviewPath, end)
	}
	return body
}

// collapsedNames は列挙の断片からパッケージ名を拾う。
func collapsedNames(t *testing.T, node, enum string) []string {
	t.Helper()

	var names []string
	for _, m := range collapsedNodeName.FindAllStringSubmatch(enum, -1) {
		// page は「`ui/page` 以下」として列挙とは別に書かれている。
		if m[1] == "page" {
			continue
		}
		names = append(names, m[1])
	}
	// 正規表現や区切り語の取り違えで 0 件になったまま緑になるのを防ぐ。
	if len(names) == 0 {
		t.Fatalf("%s の散文から %s に畳んだパッケージを 1 件も拾えない", componentOverviewPath, node)
	}
	return names
}

// collapsedNodeImpl は実装の internal/ui 直下のサブパッケージを畳み先ごとに返す。
func collapsedNodeImpl(t *testing.T) (app, parts []string) {
	t.Helper()

	for _, p := range modulePackages(t) {
		rel := relPackage(p.ImportPath)
		name, ok := strings.CutPrefix(rel, "internal/ui/")
		if !ok || strings.Contains(name, "/") {
			continue // internal/ui の直下だけを見る（散文が挙げているのはこの粒度）
		}
		node, _ := nodeForPackage(rel)
		switch {
		case node == "UIApp" && name != "page":
			// page は「`ui/page` 以下」として列挙とは別に書かれている。
			app = append(app, name)
		case node == "UIParts":
			parts = append(parts, name)
		}
	}
	return app, parts
}

// compareCollapsedNames は実装と散文の列挙を両方向で突き合わせる。
func compareCollapsedNames(t *testing.T, node string, impl, prose []string) {
	t.Helper()

	for _, name := range impl {
		if !slices.Contains(prose, name) {
			t.Errorf("%s の散文の列挙に ui/%s が無い（実装には在る）", node, name)
		}
	}
	for _, name := range prose {
		if !slices.Contains(impl, name) {
			t.Errorf("%s の散文が ui/%s を挙げているが実装に無い", node, name)
		}
	}
}
