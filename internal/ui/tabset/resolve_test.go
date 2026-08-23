package tabset

import (
	"strconv"
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/ui/page/pagetest"
)

// IndexOfTitle は名前が一致しないと ok を偽で返す。移動先が見つからない要求を
// 親が黙って無視できるのはこの否定側の契約があるからである（ui.openTab）。
func TestIndexOfTitleNotFound(t *testing.T) {
	tabs := newTestTabs(pagetest.Caps())
	if _, ok := IndexOfTitle(tabs, "存在しないタブ"); ok {
		t.Error("存在しない名前で ok が真になった")
	}
}

// タブの移動は端で折り返し、実装されていない番号キーは IndexOfKey が ok を偽で
// 返す（ui.moveTab / ui.selectTab）。
//
// 期待値をタブの枚数から計算するのは、タブを 1 枚足したときにこのテストを
// 書き換えずに済むようにするためである。
func TestNextWrapsAtEnds(t *testing.T) {
	tabs := newTestTabs(pagetest.Caps())
	last := len(tabs) - 1 // この版は 7 枚すべて有効。

	if next, ok := Next(tabs, 0, 1); !ok || next != 1 {
		t.Errorf("Next(0, +1) = (%d, %v), want (1, true)", next, ok)
	}
	if next, ok := Next(tabs, 0, -1); !ok || next != last {
		t.Errorf("Next(0, -1) = (%d, %v), want (%d, true)（末尾へ折り返す）", next, ok, last)
	}

	missing := strconv.Itoa(len(tabs) + 1)
	if _, ok := IndexOfKey(tabs, missing); ok {
		t.Errorf("存在しない番号キー %q で ok が真になった", missing)
	}
}

// Next は無効なタブを飛ばす。有効なタブが 1 枚しか無ければそこへ留まる
// （ui.moveTab）。IndexOfKey は有効・無効を見ずに添字を引くため、無効なタブへ
// 移らせない判断は呼び出し側（Tab.Enabled）が担う（ui.selectTab）。
func TestNextSkipsDisabledTabs(t *testing.T) {
	tabs := newTestTabs(pagetest.Caps())
	for i := 1; i < len(tabs); i++ {
		tabs[i].Enabled = false
	}

	if next, ok := Next(tabs, 0, 1); !ok || next != 0 {
		t.Errorf("Next(0, +1) = (%d, %v), want (0, true)（唯一の有効タブに留まる）", next, ok)
	}
	if idx, ok := IndexOfKey(tabs, tabs[1].Key); !ok || idx != 1 {
		t.Errorf("IndexOfKey(%q) = (%d, %v), want (1, true)", tabs[1].Key, idx, ok)
	}
}
