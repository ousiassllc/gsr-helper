package keymap

import (
	"slices"
	"testing"

	"charm.land/bubbles/v2/key"
)

// keysOf は Binding の並びから実際のキー文字列を取り出す。
func keysOf(bs []key.Binding) []string {
	out := make([]string, 0, len(bs))
	for _, b := range bs {
		out = append(out, b.Keys()[0])
	}
	return out
}

// フッタ 1 行目の並びと短い表記は screens.md の共通レイアウトそのものである。
//
// 表記を Binding の説明文（ヘルプ用の長い表記）と共有すると、幅 80 のフッタに操作キーが
// 半分も収まらない。集合・並び・文言を仕様側に固定する。
func TestFooterMatchesSpec(t *testing.T) {
	want := []struct{ key, desc string }{
		{"s", "開始"}, {"x", "停止"}, {"X", "強制"}, {"d", "ドレイン"}, {"D", "削除"},
		{"n", "追加"}, {"u", "更新"}, {"e", "設定"}, {"l", "ログ"},
	}

	footer := NewRunnerKeys().Footer()
	if len(footer) != len(want) {
		t.Fatalf("フッタの操作 = %d 件, want %d 件", len(footer), len(want))
	}
	for i, f := range footer {
		if got := f.Binding.Keys()[0]; got != want[i].key {
			t.Errorf("%d 番目のキー = %q, want %q", i, got, want[i].key)
		}
		if f.Desc != want[i].desc {
			t.Errorf("キー %q の表記 = %q, want %q", want[i].key, f.Desc, want[i].desc)
		}
	}
}

// 詳細画面の操作リストの並びは screens.md の詳細画面のキー表そのものである。
//
// n（追加）は対象となる runner を持たない操作なので載せない。E（切替）は載せる。
// フッタには幅の都合で出せず、ここと ? の全キー一覧だけが辿れる経路になる。
func TestDetailMatchesSpec(t *testing.T) {
	want := []string{"l", "s", "d", "R", "E", "e", "u", "x", "X", "D"}
	if got := keysOf(NewRunnerKeys().Detail()); !slices.Equal(got, want) {
		t.Errorf("詳細画面の操作 = %v, want %v", got, want)
	}
}

// Jobs タブのフッタは screens.md の Jobs タブの 4 操作を Footer と同じ表記で返す。
//
// 削除（D）と設定編集（e）を出さないのは、この 2 つを詳細画面から辿る操作としている
// ためである。表記を Footer と共有するのは、同じキーの説明文が画面によって食い違わない
// ようにするためである（page 側で文言を書き直させない）。
func TestJobsFooterMatchesSpec(t *testing.T) {
	r := NewRunnerKeys()
	want := []struct{ key, desc string }{
		{"d", "ドレイン"}, {"X", "強制"}, {"R", "再起動"}, {"l", "ログ"},
	}

	shared := make(map[string]string, len(r.Footer()))
	for _, f := range r.Footer() {
		shared[f.Binding.Keys()[0]] = f.Desc
	}

	jobs := r.JobsFooter()
	if len(jobs) != len(want) {
		t.Fatalf("Jobs タブのフッタ = %d 件, want %d 件", len(jobs), len(want))
	}
	for i, f := range jobs {
		k := f.Binding.Keys()[0]
		if k != want[i].key || f.Desc != want[i].desc {
			t.Errorf("%d 番目 = %q/%q, want %q/%q", i, k, f.Desc, want[i].key, want[i].desc)
		}
		if d, ok := shared[k]; ok && d != f.Desc {
			t.Errorf("キー %q の表記 = %q, Footer の %q と食い違う", k, f.Desc, d)
		}
	}
}

// フッタと詳細に載るのは Bindings にある操作だけである（打てないキーを出さない）。
func TestFooterAndDetailAreSubsetsOfBindings(t *testing.T) {
	r := NewRunnerKeys()
	all := keysOf(r.Bindings())

	footer := make([]string, 0, len(r.Footer()))
	for _, f := range r.Footer() {
		footer = append(footer, f.Binding.Keys()[0])
	}
	jobs := make([]string, 0, len(r.JobsFooter()))
	for _, f := range r.JobsFooter() {
		jobs = append(jobs, f.Binding.Keys()[0])
	}
	for name, keys := range map[string][]string{
		"Footer": footer, "JobsFooter": jobs, "Detail": keysOf(r.Detail()),
	} {
		for _, k := range keys {
			if !slices.Contains(all, k) {
				t.Errorf("%s のキー %q が Bindings に無い", name, k)
			}
		}
	}
}
