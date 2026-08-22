package keymap

import (
	"reflect"
	"testing"

	"charm.land/bubbles/v2/key"
)

// named は検証対象の Binding に名前を付けた組。
type named struct {
	name    string
	binding key.Binding
}

// allBindings は全キー定義を名前付きで返す。
func allBindings(s Set) []named {
	return []named{
		{"Global.TabSelect", s.Global.TabSelect},
		{"Global.TabNext", s.Global.TabNext},
		{"Global.TabPrev", s.Global.TabPrev},
		{"Global.Refresh", s.Global.Refresh},
		{"Global.Help", s.Global.Help},
		{"Global.Quit", s.Global.Quit},
		{"Global.Interrupt", s.Global.Interrupt},
		{"Global.Back", s.Global.Back},
		{"List.Up", s.List.Up},
		{"List.Down", s.List.Down},
		{"List.Top", s.List.Top},
		{"List.Bottom", s.List.Bottom},
		{"List.PageDown", s.List.PageDown},
		{"List.PageUp", s.List.PageUp},
		{"List.Toggle", s.List.Toggle},
		{"List.SelectAll", s.List.SelectAll},
		{"List.Filter", s.List.Filter},
		{"List.Accept", s.List.Accept},
		{"List.Cancel", s.List.Cancel},
		{"List.Enter", s.List.Enter},
		{"Runner.Start", s.Runner.Start},
		{"Runner.Stop", s.Runner.Stop},
		{"Runner.Kill", s.Runner.Kill},
		{"Runner.Drain", s.Runner.Drain},
		{"Runner.Restart", s.Runner.Restart},
		{"Runner.Enable", s.Runner.Enable},
		{"Runner.Add", s.Runner.Add},
		{"Runner.Delete", s.Runner.Delete},
		{"Runner.Update", s.Runner.Update},
		{"Runner.Edit", s.Runner.Edit},
		{"Runner.Logs", s.Runner.Logs},
	}
}

// キーの説明文が無いと、フッタとヘルプで文言が食い違う余地が残る。
func TestEveryBindingHasKeysAndHelp(t *testing.T) {
	for _, b := range allBindings(New()) {
		if len(b.binding.Keys()) == 0 {
			t.Errorf("%s にキーが割り当てられていない", b.name)
		}
		if !b.binding.Enabled() {
			t.Errorf("%s が初期状態で無効になっている", b.name)
		}
		if b.binding.Help().Key == "" {
			t.Errorf("%s にヘルプのキー表記が無い", b.name)
		}
		if b.binding.Help().Desc == "" {
			t.Errorf("%s に説明文が無い", b.name)
		}
	}
}

// 定義の総数を固定し、Binding を追加したときにテストから漏れることを防ぐ。
func TestAllBindingsCoversEveryField(t *testing.T) {
	s := New()
	want := reflect.TypeOf(s.Global).NumField() +
		reflect.TypeOf(s.List).NumField() +
		reflect.TypeOf(s.Runner).NumField()
	if got := len(allBindings(s)); got != want {
		t.Fatalf("検証対象の件数 = %d, want %d（Binding を追加したらテストも追う）", got, want)
	}
}

// screens.md のキーマップ表と割り当てが一致する。
func TestKeyAssignmentsMatchSpec(t *testing.T) {
	want := map[string][]string{
		"Global.TabSelect": {"1", "2", "3", "4", "5", "6", "7"},
		"Global.TabNext":   {"tab"},
		"Global.TabPrev":   {"shift+tab"},
		"Global.Refresh":   {"r"},
		"Global.Help":      {"?"},
		"Global.Quit":      {"q"},
		"Global.Interrupt": {"ctrl+c"},
		"Global.Back":      {"esc"},
		"List.Up":          {"k", "up"},
		"List.Down":        {"j", "down"},
		"List.Top":         {"g"},
		"List.Bottom":      {"G"},
		"List.PageDown":    {"ctrl+f"},
		"List.PageUp":      {"ctrl+b"},
		"List.Toggle":      {"space"},
		"List.SelectAll":   {"ctrl+a"},
		"List.Filter":      {"/"},
		"List.Accept":      {"enter"},
		"List.Cancel":      {"esc"},
		"List.Enter":       {"enter"},
		"Runner.Start":     {"s"},
		"Runner.Stop":      {"x"},
		"Runner.Kill":      {"X"},
		"Runner.Drain":     {"d"},
		"Runner.Restart":   {"R"},
		"Runner.Enable":    {"E"},
		"Runner.Add":       {"n"},
		"Runner.Delete":    {"D"},
		"Runner.Update":    {"u"},
		"Runner.Edit":      {"e"},
		"Runner.Logs":      {"l"},
	}
	for _, b := range allBindings(New()) {
		if !reflect.DeepEqual(b.binding.Keys(), want[b.name]) {
			t.Errorf("%s のキー = %v, want %v", b.name, b.binding.Keys(), want[b.name])
		}
	}
}

// enter と esc の共有は「特別扱いの除外」ではなく、モードの分割で成り立っている。
//
// List.Accept（enter）と List.Enter（enter）、List.Cancel（esc）と Global.Back（esc）は
// 同じキーを使うが、有効になるモードが排他である（入力中はグローバルキーを解釈せず、
// 通常時は Accept / Cancel を解釈しない。atomic-design.md のキー入力の配送）。
// そこで**キーを分けるのではなく、モードの分割そのものを固定する。** List の全
// フィールドが Bindings と FilterBindings のどちらか一方だけに現れることを見れば、
// 「どちらのモードにも属さないキー」も「両方に属するキー」も作れない（Issue #50）。
func TestListModesPartitionEveryKey(t *testing.T) {
	l := NewList()

	seen := make(map[string]int, reflect.TypeOf(l).NumField())
	for _, b := range append(l.Bindings(), l.FilterBindings()...) {
		seen[b.Help().Desc]++
	}

	v := reflect.ValueOf(l)
	for i := range v.NumField() {
		b, ok := v.Field(i).Interface().(key.Binding)
		if !ok {
			t.Fatalf("%s は key.Binding ではない", reflect.TypeOf(l).Field(i).Name)
		}
		switch seen[b.Help().Desc] {
		case 1:
		case 0:
			t.Errorf("%s がどちらのモードにも属していない", reflect.TypeOf(l).Field(i).Name)
		default:
			t.Errorf("%s が両方のモードに属している", reflect.TypeOf(l).Field(i).Name)
		}
	}
}

// Order は表示順を決めるだけであり、操作を取りこぼしてはならない。
func TestRunnerOrderContainsEveryActionOnce(t *testing.T) {
	r := NewRunnerKeys()
	count := make(map[string]int)
	for _, b := range r.Order() {
		count[b.Help().Key]++
	}

	for _, b := range r.Bindings() {
		switch n := count[b.Help().Key]; n {
		case 1:
		case 0:
			t.Errorf("Order に %q（%s）が無い", b.Help().Key, b.Help().Desc)
		default:
			t.Errorf("Order に %q が %d 回現れている", b.Help().Key, n)
		}
	}
	if len(r.Order()) != len(r.Bindings()) {
		t.Errorf("Order の件数 = %d, want %d", len(r.Order()), len(r.Bindings()))
	}
}

// 破壊的な操作は後に置く（詳細画面で区切り線の下にまとめるため）。
func TestRunnerOrderPutsDestructiveLast(t *testing.T) {
	order := NewRunnerKeys().Order()
	index := make(map[string]int, len(order))
	for i, b := range order {
		index[b.Help().Key] = i
	}
	for _, safe := range []string{"l", "s", "d", "R", "E", "e", "u", "n"} {
		for _, destructive := range []string{"x", "X", "D"} {
			if index[safe] > index[destructive] {
				t.Errorf("安全な操作 %q が破壊的な操作 %q より後にある", safe, destructive)
			}
		}
	}
}

// 呼び出しごとに新しい値を返す（片方を無効化しても他に影響しない）。
func TestConstructorsReturnFreshValues(t *testing.T) {
	g := NewGlobal()
	g.Quit.SetEnabled(false)
	if !NewGlobal().Quit.Enabled() {
		t.Error("NewGlobal の返り値への変更が次の呼び出しに影響している")
	}

	l := NewList()
	l.Up.SetKeys("z")
	if got := NewList().Up.Keys(); reflect.DeepEqual(got, []string{"z"}) {
		t.Error("NewList の返り値への変更が次の呼び出しに影響している")
	}

	r := NewRunnerKeys()
	r.Stop.SetEnabled(false)
	if !NewRunnerKeys().Stop.Enabled() {
		t.Error("NewRunnerKeys の返り値への変更が次の呼び出しに影響している")
	}

	s := New()
	s.Runner.Delete.SetEnabled(false)
	if !New().Runner.Delete.Enabled() {
		t.Error("New の返り値への変更が次の呼び出しに影響している")
	}

	// Bindings / Order の返り値を書き換えても定義に影響しない。
	bindings := NewRunnerKeys().Bindings()
	bindings[0].SetKeys("z")
	if got := NewRunnerKeys().Bindings()[0].Keys(); reflect.DeepEqual(got, []string{"z"}) {
		t.Error("Bindings の返り値への変更が次の呼び出しに影響している")
	}
}
