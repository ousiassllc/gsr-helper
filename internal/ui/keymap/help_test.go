package keymap

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/key"
)

// ? の全キー一覧（Set.Help / RunnerListHelp）の検証を集める。

func TestFullHelpGroupsAreNotEmpty(t *testing.T) {
	groups := New().RunnerListHelp()
	if len(groups) == 0 {
		t.Fatal("RunnerListHelp が空である")
	}
	for i, g := range groups {
		if len(g) == 0 {
			t.Errorf("RunnerListHelp のグループ %d が空である", i)
		}
		for _, b := range g {
			if b.Help().Key == "" || b.Help().Desc == "" {
				t.Errorf("RunnerListHelp のグループ %d に説明文の無いキーがある", i)
			}
		}
	}
}

// ? の中身は画面が渡したグループだけで決まる（Global は常に先頭）。
//
// 画面ごとに範囲を渡させないと、Disk / Logs / Config のキーが Runners のヘルプにも
// 並ぶ（Set にキーの種類が増えるたびに全画面のヘルプが太る）。
func TestHelpContainsOnlyGivenGroups(t *testing.T) {
	s := New()
	own := []key.Binding{key.NewBinding(key.WithKeys("D"), key.WithHelp("D", "この画面だけのキー"))}

	groups := s.Help(own)
	if len(groups) != 2 {
		t.Fatalf("グループ数 = %d, want 2（Global + 渡した 1 つ）", len(groups))
	}
	if got := groups[0][0].Help().Key; got != s.Global.TabSelect.Help().Key {
		t.Errorf("先頭のグループ = %q, want Global", got)
	}
	if got := groups[1][0].Help().Desc; got != "この画面だけのキー" {
		t.Errorf("2 つ目のグループ = %q, want 渡したグループ", got)
	}

	// 渡していない runner の操作キーは出ない。
	for _, g := range groups {
		for _, b := range g {
			if b.Help().Desc == s.Runner.Drain.Help().Desc {
				t.Error("渡していない runner の操作キーが ? に出ている")
			}
		}
	}

	// 空のグループは足さない（bubbles/help が空の列を描かないようにする）。
	if got := len(s.Help(nil, own)); got != 2 {
		t.Errorf("空グループを渡したときのグループ数 = %d, want 2", got)
	}
}

// enter と esc は「通常モード」と「絞り込み中」でグループが分かれる。
//
// 1 つのグループに混ぜると、? の一覧に同じキーが違う説明で 2 行並び、どちらが
// 効くのか読み取れない（Accept 確定 と Enter 詳細を開く、Cancel 取消 と Back 戻る）。
func TestFilteringKeysAreASeparateHelpGroup(t *testing.T) {
	s := New()

	normal := s.List.Bindings()
	for _, b := range normal {
		for _, k := range b.Keys() {
			if k == "enter" && b.Help().Desc != s.List.Enter.Help().Desc {
				t.Errorf("通常モードのグループに絞り込み中のキー（%s）がある", b.Help().Desc)
			}
		}
	}

	filtering := s.List.FilterBindings()
	if len(filtering) != 2 {
		t.Fatalf("絞り込み中のキー数 = %d, want 2（確定 / 取消）", len(filtering))
	}
	for _, b := range filtering {
		if !strings.Contains(b.Help().Desc, "絞り込み") {
			t.Errorf("絞り込み中のキーの説明 = %q, want どの状態で効くか分かる文", b.Help().Desc)
		}
	}
}
