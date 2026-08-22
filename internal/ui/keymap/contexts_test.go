package keymap

// Set.Contexts（同時に有効なキーの集合）に関する検査をまとめる。
//
// keymap_test.go から分けているのは 1 ファイル 300 行の上限に収めるためで、
// 同じディレクトリなので keymap ディレクトリ全体の行数は変わらない（Issue #35）。

import (
	"reflect"
	"testing"

	"charm.land/bubbles/v2/key"
)

// 同じ画面で同時に有効なキーが重複していると、打鍵が別の操作として解釈される。
//
// 検査するコンテキストの一覧はテストではなく Set.Contexts が持つ。ここへ書き写すと
// 「実装は増えたのにテスト側の一覧だけが古い」形を作れてしまい、検査が静かに
// 素通りする（Issue #40）。
func TestNoDuplicateKeysInSameContext(t *testing.T) {
	for _, c := range New().Contexts() {
		assertNoDuplicateKeys(t, c.Name, c.Keys)
	}
}

// Set のどのフィールドも、少なくとも 1 つのコンテキストに登録されている。
//
// 重複検査が意味を持つ根拠は「同時に有効なキーの集合を漏れなく列挙していること」
// だけである。新しいタブが自前のキー集合を Set に足したとき、Contexts への登録を
// 忘れるとそのキーは一度も検査されないまま親のグローバルキー（r / q / 1〜7 / tab）と
// 衝突しうる。衝突すれば 1 打鍵で page と親が両方動く。
//
// 人の注意ではなく reflect で登録を強制する（既存の TestAllBindingsCoversEveryField と
// 同じ手法を Set 全体へ広げたもの）。
func TestContextsCoverEverySetField(t *testing.T) {
	s := New()

	registered := make(map[string]bool)
	for _, c := range s.Contexts() {
		for _, f := range c.Fields {
			registered[f] = true
		}
	}

	st := reflect.TypeOf(s)
	for i := range st.NumField() {
		if name := st.Field(i).Name; !registered[name] {
			t.Errorf("Set.%s がどのコンテキストにも登録されていない"+
				"（keymap.Set.Contexts へ、そのキーが同時に有効になるコンテキストを足すこと）", name)
		}
	}
}

// Contexts の記載自体が Set の実態からずれていない。
//
// Fields は文字列なので、綴りを間違えたり消したフィールドを残したりすると
// TestContextsCoverEverySetField の網羅判定が空振りする。実在と非空を別に見る。
func TestContextsAreWellFormed(t *testing.T) {
	st := reflect.TypeOf(New())
	for _, c := range New().Contexts() {
		if c.Name == "" {
			t.Error("名前の無いコンテキストがある")
		}
		if len(c.Fields) == 0 {
			t.Errorf("コンテキスト %q に由来フィールドの記載が無い", c.Name)
		}
		if len(c.Keys) == 0 {
			t.Errorf("コンテキスト %q にキーが 1 つも無い", c.Name)
		}
		for _, f := range c.Fields {
			if _, ok := st.FieldByName(f); !ok {
				t.Errorf("コンテキスト %q の由来フィールド %q は Set に存在しない", c.Name, f)
			}
		}
	}
}

// 一覧の通常モードのキー集合は、入力中にしか使わない 2 つを除いた List 全体である。
//
// 「同時に有効なキーの集合」を狭く取ると重複検査が働かない。List にフィールドを
// 増やしたときに除外の判断を迫るため、件数を突き合わせる。
func TestListNormalContextCoversEveryNonFilterKey(t *testing.T) {
	l := NewList()

	got := len(l.Bindings())
	if want := reflect.TypeOf(l).NumField() - len(l.FilterBindings()); got != want {
		t.Fatalf("通常モードのキー数 = %d, want %d（List にキーを追加したらテストも追う）", got, want)
	}
}

func assertNoDuplicateKeys(t *testing.T, context string, bindings []key.Binding) {
	t.Helper()

	seen := make(map[string]string)
	for _, b := range bindings {
		for _, k := range b.Keys() {
			if prev, ok := seen[k]; ok {
				t.Errorf("%s: キー %q が %q と %q で重複している", context, k, prev, b.Help().Desc)
				continue
			}
			seen[k] = b.Help().Desc
		}
	}
}
