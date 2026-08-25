package docscheck

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"github.com/ousiassllc/gsr-helper/internal/buildconfig/buildconfigtest"
)

// この表の各行の説明が、その行が挙げるテストの doc コメントの要約段落（最初の空行まで）と
// 一致する。不変条件を述べるのは doc コメントだけとし、表はその写しに徹する——ずれたときに
// 直すのは表の側である。
//
// TestSetupDocInvariantTableListsEveryTest が機械的に守っているのは**テスト名の集合だけ**で、
// 各行が述べる「守っている不変条件」そのものは検査の外にあった。PR #173 のレビューは、その説明が
// 実装と食い違う欠陥を 2 周で計 10 行ぶん見つけている——nolintlint の行が設定にも検査にも無い
// 「行単位」の強制を述べ、実際に検査している `allow-unused: false` は落ちていた、といった形である。
// **表が「網羅である」と宣言して読み手の一次情報になった以上、説明の過大申告は危険側である**
// ——読み手は「この不変条件は機械的に守られている」と信じて設定を触る。
//
// そこで**写しを 1 つに減らした**。不変条件を述べるのはテストの doc コメントだけとし、表の説明は
// その要約段落（最初の空行まで）の写しに徹する。Issue #164 / #167 / #169 が行数の実測値に対して採ったのと同じ手で、
// **一致を見るのだから照合は完全一致でよい**——「設定キーが説明に現れているか」のような弱い検査を
// 組み立てる必要も、言い回しを列挙する必要も無い。ずれたときに直すのは表の側である。
//
// **1 行 1 検査にしてあるのは、この一致を定義できるようにするためである。** 2 つのテストを 1 行に
// まとめていた頃は、行の説明がどちらの doc コメントに対応するのかが決まらなかった。
func TestSetupDocInvariantTableDescriptionsMatchDocComments(t *testing.T) {
	root := buildconfigtest.RepoRoot(t)

	want := invariantTestDocSummaries(t, root)
	for _, row := range invariantTableRows(t, root) {
		if len(row.tests) != 1 {
			t.Errorf("setup.md:%d: 一覧表の行が挙げるテストが %d 件ある（1 行 1 検査にすること）",
				row.line, len(row.tests))
			continue
		}
		name := row.tests[0]
		summary, ok := want[name]
		if !ok {
			// 実装に無いテストを挙げている場合は集合の一致の検査が報告する。
			continue
		}
		if summary == "" {
			t.Errorf("%s に doc コメントが無い（何を守る検査かを要約段落に書くこと）", name)
			continue
		}
		if row.desc != summary {
			t.Errorf("setup.md:%d: %s の行の説明が doc コメントの要約段落と一致しない。"+
				"直すのは表の側である\n  表:       %s\n  コメント: %s", row.line, name, row.desc, summary)
		}
	}
}

// invariantTestDocSummaries はテスト関数名から doc コメントの要約段落を引く。
//
// 要約段落は doc コメントの**最初の空行まで**である。1 文目だけを取らないのは、この節の
// 検査が「何を守るか」に続けて短い理由を 1 文添える形で揃っているためで、表の説明も
// 同じ 2 文を持っていた。理由の詳細（破られたときに何が起きるか）は空行より後ろにあり、
// 表には持たせない。
//
// doc コメントは日本語を桁で折り返すので、改行は詰めて 1 行に戻す。**ただし折り返しの
// 前後がどちらも英数字のときだけ空白を挟む**——`persist-credentials` のような識別子が
// 行をまたいだときに、詰めると語が繋がってしまうためである。
func invariantTestDocSummaries(t *testing.T, root string) map[string]string {
	t.Helper()

	fset := token.NewFileSet()
	summaries := make(map[string]string)
	for _, path := range invariantTestFiles(t, root) {
		file, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if err != nil {
			t.Fatalf("Go ソースとして解析できない（%s）: %v", path, err)
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || !isTestFuncDecl(fn) {
				continue
			}
			summaries[fn.Name.Name] = docSummary(fn.Doc)
		}
	}
	return summaries
}

// docSummary は doc コメントの要約段落を 1 行に詰めて返す。コメントが無ければ空文字を返す。
func docSummary(doc *ast.CommentGroup) string {
	if doc == nil {
		return ""
	}
	para, _, _ := strings.Cut(doc.Text(), "\n\n")
	var b strings.Builder
	for _, line := range strings.Split(strings.TrimSpace(para), "\n") {
		if b.Len() > 0 && isASCIIWordBoundary(b.String(), line) {
			b.WriteString(" ")
		}
		b.WriteString(line)
	}
	return b.String()
}

// isASCIIWordBoundary は詰めた行の末尾と次の行の先頭がどちらも英数字かどうかを返す。
func isASCIIWordBoundary(joined, next string) bool {
	if next == "" {
		return false
	}
	prev, _ := utf8.DecodeLastRuneInString(joined)
	head, _ := utf8.DecodeRuneInString(next)
	isWord := func(r rune) bool {
		return r < utf8.RuneSelf && (unicode.IsLetter(r) || unicode.IsDigit(r))
	}
	return isWord(prev) && isWord(head)
}
