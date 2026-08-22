package jobs_test

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// / で絞り込みが始まり、入力中であることを親へ報告する。
func TestFilterReportsInput(t *testing.T) {
	m, _ := newModel(t, busyRunner("build01-1", 1), busyRunner("build01-7", 1))

	m, cmd := m.Update(press("/"))
	c := chrome(t, cmd)
	if c.Input != "絞り込み" {
		t.Fatalf("Input = %q, want 絞り込み", c.Input)
	}
	if !strings.Contains(c.Status, "入力中") {
		t.Errorf("状態行 = %q, 入力中を示していない", c.Status)
	}

	// 入力中はグローバルキーを解釈せず、打った文字が入力欄へ入る。
	m, cmd = m.Update(press("7"))
	if c = chrome(t, cmd); c.Input == "" {
		t.Fatal("入力中に数字を打つと入力が終わっている")
	}
	if got := m.View().Content; !strings.Contains(got, "build01-7") || strings.Contains(got, "build01-1") {
		t.Errorf("絞り込みの結果が反映されていない: %q", got)
	}

	// esc は入力を取り消す。
	m, cmd = m.Update(press("esc"))
	if c = chrome(t, cmd); c.Input != "" {
		t.Error("esc で入力が終わらない")
	}
	if got := m.View().Content; !strings.Contains(got, "build01-1") {
		t.Error("絞り込みが解除されていない")
	}
}

// ? でヘルプを開き、モーダル表示中は最上位にのみキーが届く。
func TestHelpModal(t *testing.T) {
	m, _ := newModel(t, busyRunner("build01-1", 1))

	m, cmd := m.Update(press("?"))
	if c := chrome(t, cmd); !c.Modal {
		t.Fatal("? でヘルプが開かない")
	}
	if got := m.View().Content; !strings.Contains(got, "ヘルプ") {
		t.Errorf("ヘルプが描かれていない: %q", got)
	}

	// タブ切替のキーはモーダルに吸われる。
	m, cmd = m.Update(press("2"))
	if c := chrome(t, cmd); !c.Modal {
		t.Error("モーダル表示中に 2 を打つと閉じている")
	}

	_, cmd = m.Update(press("esc"))
	if c := chrome(t, cmd); c.Modal {
		t.Error("esc でモーダルが閉じない")
	}
}

// キー以外の Msg も配られ、状態を壊さない（点滅などの Cmd を止めないため）。
func TestForwardsNonKeyMessages(t *testing.T) {
	m, _ := newModel(t, busyRunner("build01-1", 1))

	m, cmd := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	if c := chrome(t, cmd); c.Modal || c.Input != "" {
		t.Errorf("キー以外の Msg で状態が変わっている: %+v", c)
	}
	if !strings.Contains(m.View().Content, "build01-1") {
		t.Error("一覧が消えている")
	}
}
