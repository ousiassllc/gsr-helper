package ui

import (
	"strconv"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/ousiassllc/gsr-helper/internal/exec"
	"github.com/ousiassllc/gsr-helper/internal/runner"
	"github.com/ousiassllc/gsr-helper/internal/ui/chrome"
	"github.com/ousiassllc/gsr-helper/internal/ui/discovery"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/pagetest"
)

// 本体以外の領域（タブ行・状態行・フッタ）の表示を検証する。

// タブ行には screens.md の共通レイアウトの 7 タブすべてが出る（幅 80 で収まる）。
func TestViewShowsEverySpecTab(t *testing.T) {
	a, _ := update(newApp(exec.NewFake()), tea.WindowSizeMsg{Width: 80, Height: 24})
	line := strings.Split(a.View().Content, "\n")[1]

	for i, want := range []string{
		"Runners", "Jobs", "Disk", "Logs", "Doctor", "Config", "Setup",
	} {
		if !strings.Contains(line, "["+strconv.Itoa(i+1)+"]"+want) &&
			!strings.Contains(line, "("+strconv.Itoa(i+1)+")"+want) {
			t.Errorf("タブ %q がタブ行に無い: %q", want, line)
		}
	}
	if w := lipgloss.Width(line); w > 80 {
		t.Errorf("タブ行の幅 = %d, want 80 以下（%q）", w, line)
	}
}

// 幅 80 のフッタ 1 行目に screens.md の共通レイアウトの操作キーがすべて出る。
//
// 設計原則 1「有効なキーを常に画面に出す」の受け入れ条件に相当する不変条件である。
// キーの説明文が長いと 11 個のうち 4 個しか収まらない状態になるため、フッタ用の
// 短い表記（keymap.RunnerKeys.Footer）が効いていることをここで固定する。
func TestFooterShowsEverySpecKeyAtWidth80(t *testing.T) {
	a, _ := update(newApp(exec.NewFake()), tea.WindowSizeMsg{Width: 80, Height: 24})
	a, cmd := update(a, discovery.Msg{
		Result: runner.Result{Runners: []runner.Runner{sampleRunner()}},
		Err:    nil,
	})
	a = applyChrome(a, cmd)

	line := strings.Split(chrome.Footer(a.chromeView()), "\n")[0]
	for _, want := range []string{
		"s:開始", "x:停止", "X:強制", "d:ドレイン", "D:削除",
		"n:追加", "u:更新", "e:設定", "l:ログ", "?:ヘルプ",
	} {
		if !strings.Contains(line, want) {
			t.Errorf("フッタ 1 行目に %q が無い: %q", want, line)
		}
	}
	if w := lipgloss.Width(line); w > 80 {
		t.Errorf("フッタ 1 行目の幅 = %d, want 80 以下（%q）", w, line)
	}

	// 7 タブすべてが実装済みなので、未対応で塞がる操作はもう無い。
	if strings.Contains(chrome.Footer(a.chromeView()), "この版では未対応です") {
		t.Error("未対応の操作が残っている")
	}
}

// 無効なキーはフッタ 2 行目に丸括弧付きで並べ、理由を添える（設計原則 4）。
func TestFooterShowsReasonForDisabledKey(t *testing.T) {
	a, _ := update(newApp(exec.NewFake()), tea.WindowSizeMsg{Width: 80, Height: 24})
	a, cmd := update(a, discovery.Msg{
		Result: runner.Result{Runners: []runner.Runner{pagetest.BusyRunner()}},
		Err:    nil,
	})

	lines := strings.Split(chrome.Footer(applyChrome(a, cmd).chromeView()), "\n")
	if len(lines) < 2 {
		t.Fatalf("フッタが 2 行に足りない: %q", lines)
	}
	if !strings.Contains(lines[1], "(D)") || !strings.Contains(lines[1], "ジョブ実行中") {
		t.Errorf("フッタ 2 行目 = %q, 無効なキーと理由が出ていない", lines[1])
	}
}
