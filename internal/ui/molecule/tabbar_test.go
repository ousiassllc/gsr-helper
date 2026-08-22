package molecule

import (
	"strconv"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/ousiassllc/gsr-helper/internal/ui/token"
)

func sampleTabs() []TabView {
	return []TabView{
		{Key: "1", Title: "Runners", Active: true, Enabled: true},
		{Key: "2", Title: "Jobs", Active: false, Enabled: true},
		{Key: "3", Title: "Disk", Active: false, Enabled: false},
	}
}

// 選択中・選択可・選択不可の 3 状態が色以外でも区別できる（設計原則 4）。
func TestTabBarDistinguishesStatesWithoutColor(t *testing.T) {
	for name, s := range map[string]token.Styles{"色なし": plainStyles(), "色あり": token.NewStyles(true, true)} {
		got := TabBar(sampleTabs(), 120, s)
		for _, want := range []string{token.IconCursor, "[1]", "[2]", "(3)"} {
			if !strings.Contains(got, want) {
				t.Errorf("%s: %q が無い: %q", name, want, got)
			}
		}
	}

	// 3 状態の表示が互いに異なる。
	labels := map[string]string{}
	for _, tab := range sampleTabs() {
		labels[tabLabel(tab, plainStyles())] = tab.Title
	}
	if len(labels) != 3 {
		t.Errorf("3 状態のうち区別できない組がある: %v", labels)
	}
}

func TestTabBarTruncatesToWidth(t *testing.T) {
	titles := []string{"Runners", "Jobs", "Disk", "Logs", "Doctor", "Config", "Setup"}
	tabs := make([]TabView, 0, len(titles))
	for i, title := range titles {
		tabs = append(tabs, TabView{Key: strconv.Itoa(i + 1), Title: title, Active: i == 0, Enabled: true})
	}
	const width = 30
	got := TabBar(tabs, width, plainStyles())

	if w := lipgloss.Width(got); w > width {
		t.Errorf("タブ行の幅 = %d, 上限 %d を超えた（%q）", w, width, got)
	}
	for _, want := range []string{token.IconEllipsis, "Runners"} {
		if !strings.Contains(got, want) {
			t.Errorf("切り詰めた行に %q が無い: %q", want, got)
		}
	}
	// 幅 0 でもタブが無くても panic しない。
	if got := TabBar(tabs, 0, plainStyles()); got != "" {
		t.Errorf("幅 0 のタブ行 = %q, want 空文字", got)
	}
	if got := TabBar(nil, width, plainStyles()); got != "" {
		t.Errorf("タブが無いときの行 = %q, want 空文字", got)
	}
}
