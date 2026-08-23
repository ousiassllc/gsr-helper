package keymap

import (
	"slices"
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

// Logs タブの ? には runner の操作キーを出さない。
//
// Logs タブに runner への操作は無く、出せば押しても何も起きないキーがヘルプに並ぶ。
// Logs タブ固有の 3 つ（tab / f / J）は必ず出す。
func TestLogsHelpHasOwnKeysWithoutRunnerActions(t *testing.T) {
	s := New()

	seen := make(map[string]bool)
	for _, g := range s.LogsHelp() {
		for _, b := range g {
			seen[b.Help().Key] = true
		}
	}

	for _, b := range s.Log.Bindings() {
		if !seen[b.Help().Key] {
			t.Errorf("Logs タブのキー %q が ? に出ていない", b.Help().Key)
		}
	}
	for _, b := range s.Runner.Bindings() {
		if seen[b.Help().Key] {
			t.Errorf("runner の操作キー %q が Logs タブの ? に出ている", b.Help().Key)
		}
	}
}

// Logs タブの ? に「次のタブ」の tab を出さない（Issue #9 のレビュー指摘 MAJOR 11）。
//
// この画面の tab はペインの切り替えであり（LogKeys.Pane の doc）、page が消費して親へ
// 差し戻さないので「次のタブ」は押しても効かない。両方を並べると ? に tab の行が
// 2 つ、しかも互いに矛盾する説明で出る。どちらが効くのか読み取れないうえ、Contexts が
// Logs 用のコンテキストから Global.TabNext を外していること・screens.md の
// 「この画面では tab が次のタブではない」という記述とも食い違う。
//
// tab を持つ binding が 1 つだけであることまで見るのは、「TabNext を消したが別の経路で
// tab が紛れ込む」形（Global に tab を持つキーが増える、Help に渡すグループが増える）を
// 将来も捕まえるためである。
func TestLogsHelpExcludesTabNextButKeepsOtherGlobals(t *testing.T) {
	s := New()

	var tabDescs []string
	seen := make(map[string]bool)
	for _, g := range s.LogsHelp() {
		for _, b := range g {
			seen[b.Help().Desc] = true
			for _, k := range b.Keys() {
				if k == "tab" {
					tabDescs = append(tabDescs, b.Help().Desc)
				}
			}
		}
	}

	if len(tabDescs) != 1 || tabDescs[0] != s.Log.Pane.Help().Desc {
		t.Errorf("Logs タブの ? に出る tab の説明 = %q, want [%q] だけ",
			tabDescs, s.Log.Pane.Help().Desc)
	}
	if seen[s.Global.TabNext.Help().Desc] {
		t.Errorf("効かないはずの %q が Logs タブの ? に出ている", s.Global.TabNext.Help().Desc)
	}

	// tab 以外のグローバルキーは Logs タブでも効くので、すべて出ていなければならない
	// （矛盾の解消のために ? からグローバルキーごと落とす、という直し方を防ぐ）。
	for _, b := range s.Global.Bindings() {
		if slices.Equal(b.Keys(), s.Global.TabNext.Keys()) {
			continue
		}
		if !seen[b.Help().Desc] {
			t.Errorf("グローバルキー %q が Logs タブの ? に出ていない", b.Help().Key)
		}
	}
}
