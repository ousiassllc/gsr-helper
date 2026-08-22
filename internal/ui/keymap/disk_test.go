package keymap

import (
	"slices"
	"testing"
)

// screens.md の Disk タブのキーマップは space / c / r の 3 つである。
//
// このうち space（選択）と r（再集計）は List.Toggle と Global.Refresh そのもので
// あり、DiskKeys では定義しない。**定義しないことを検査で固定する。** 同じキーを
// 重ねて定義すると Disk タブのコンテキストで Binding が 2 つになり、1 打鍵で
// 選択と再集計が二重に走る経路ができる（Set.Contexts の doc）。
func TestDiskKeysDoNotRedefineSharedKeys(t *testing.T) {
	s := New()

	for _, b := range s.Disk.Bindings() {
		for _, k := range b.Keys() {
			if slices.Contains(s.List.Toggle.Keys(), k) {
				t.Errorf("Disk が List.Toggle と同じキー %q を定義している", k)
			}
			if slices.Contains(s.Global.Refresh.Keys(), k) {
				t.Errorf("Disk が Global.Refresh と同じキー %q を定義している", k)
			}
		}
	}

	// 仕様の 3 つが Disk タブのコンテキストで打てることは確かめる（定義元は問わない）。
	for _, want := range []string{"space", "c", "r"} {
		if !contextHasKey(s, "Disk タブ（通常モード）", want) {
			t.Errorf("Disk タブでキー %q が有効になっていない", want)
		}
	}
}

// 確認ダイアログで効くのは y / n / esc / enter / ctrl+c だけである
// （screens.md の確認ダイアログ）。
//
// esc と enter は Global.Back / List.Accept を使い回す。ConfirmKeys で再定義すると
// 同時に有効な Binding が 2 つになるため、ここでも「定義しないこと」を固定する。
func TestConfirmContextHasExactlyTheSpecKeys(t *testing.T) {
	s := New()

	for _, b := range s.Confirm.Bindings() {
		for _, k := range b.Keys() {
			if k == "esc" || k == "enter" {
				t.Errorf("ConfirmKeys が %q を再定義している（Global.Back / List.Accept を使う）", k)
			}
		}
	}

	want := []string{"y", "n", "esc", "enter", "ctrl+c"}
	got := contextKeys(s, "確認ダイアログ")
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Errorf("確認ダイアログで有効なキー = %v, want %v", got, want)
	}
}

// enter はキャンセル側である（設計原則 5「Enter の連打では進まない」）。
//
// 実行側（y）と同じキーになっていないことを見るだけでは足りない。キャンセルを担う
// 3 つのキーのどれかが実行側へ移っていないことを確かめる。
func TestConfirmEnterIsNotOnTheExecuteSide(t *testing.T) {
	s := New()
	for _, k := range s.Confirm.Yes.Keys() {
		if k == "enter" || k == "esc" || slices.Contains(s.Confirm.No.Keys(), k) {
			t.Errorf("実行のキーに %q が割り当てられている（キャンセル側でなければならない）", k)
		}
	}
}

// Disk タブの ? には、runner の操作キーではなく Disk 固有のキーが並ぶ。
func TestDiskHelpCarriesDiskKeysOnly(t *testing.T) {
	s := New()
	groups := s.DiskHelp()

	if len(groups) != 4 {
		t.Fatalf("グループ数 = %d, want 4（Global + 一覧 + 絞り込み中 + Disk）", len(groups))
	}
	for _, g := range groups {
		for _, b := range g {
			if b.Help().Desc == s.Runner.Drain.Help().Desc {
				t.Error("Disk タブの ? に runner の操作キーが並んでいる")
			}
		}
	}
	if got := groups[3][0].Help().Key; got != s.Disk.Clean.Help().Key {
		t.Errorf("最後のグループの先頭 = %q, want %q", got, s.Disk.Clean.Help().Key)
	}
}

// 呼び出しごとに新しい値を返す（片方を無効化しても他に影響しない）。
func TestDiskAndConfirmConstructorsReturnFreshValues(t *testing.T) {
	d := NewDiskKeys()
	d.Clean.SetEnabled(false)
	if !NewDiskKeys().Clean.Enabled() {
		t.Error("NewDiskKeys の返り値への変更が次の呼び出しに影響している")
	}

	c := NewConfirmKeys()
	c.Yes.SetEnabled(false)
	if !NewConfirmKeys().Yes.Enabled() {
		t.Error("NewConfirmKeys の返り値への変更が次の呼び出しに影響している")
	}
}

// contextKeys は名前で引いたコンテキストで有効なキー文字列を返す。
func contextKeys(s Set, name string) []string {
	var keys []string
	for _, c := range s.Contexts() {
		if c.Name != name {
			continue
		}
		for _, b := range c.Keys {
			keys = append(keys, b.Keys()...)
		}
	}
	return keys
}

// contextHasKey はコンテキストでそのキーが有効かを返す。
func contextHasKey(s Set, name, k string) bool {
	return slices.Contains(contextKeys(s, name), k)
}
