package runners_test

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/runner"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/runners"
	"github.com/ousiassllc/gsr-helper/internal/ui/token"
)

// StateMsg を受けると runner の区画と孤児ユニットの区画の両方に行が入る。
func TestStateFillsBothSections(t *testing.T) {
	m, _ := newModel(t, 80, 16)
	got := m.View().Content
	for _, want := range []string{"build01-1", "build01-2", "孤児ユニット", "old01.service"} {
		if !strings.Contains(got, want) {
			t.Errorf("%q が表示に無い", want)
		}
	}
}

// 検出結果が空のときは、その旨を出す。
func TestEmptyResult(t *testing.T) {
	st := testState(80, 16)
	st.Result = runner.Result{}
	m, _ := runners.New(0, st).Update(st)
	if got := m.View().Content; !strings.Contains(got, "runner が見つかりません") {
		t.Errorf("表示 = %q, runner が無い旨を出していない", got)
	}
}

// page は検出を自分で行わない。StateMsg を連続で受けても ChromeMsg だけを返す。
func TestStateMsgEmitsOnlyChrome(t *testing.T) {
	m := tea.Model(runners.New(0, testState(80, 16)))
	for range 3 {
		var cmd tea.Cmd
		m, cmd = m.Update(testState(80, 16))
		msgs := collect(cmd)
		if len(msgs) != 1 {
			t.Fatalf("発行された Msg の件数 = %d, want 1", len(msgs))
		}
		if _, ok := msgs[0].(page.ChromeMsg); !ok {
			t.Errorf("発行された Msg = %T, want page.ChromeMsg", msgs[0])
		}
	}
}

// enter で詳細画面が開き、モーダル表示中はグローバルキーを解釈しない。
func TestEnterOpensModalAndKeepsIt(t *testing.T) {
	m, c := newModel(t, 80, 16)
	if c.Modal {
		t.Fatal("初期状態でモーダルが開いている")
	}

	m, c = send(t, m, "enter")
	if !c.Modal {
		t.Fatal("enter で詳細画面が開かない")
	}
	if !strings.Contains(m.View().Content, "詳細") {
		t.Error("詳細画面が描かれていない")
	}

	// タブ切替のキーを打ってもモーダルは維持される（背後へ流れない）。
	m, c = send(t, m, "2")
	if !c.Modal {
		t.Error("モーダル表示中に 2 を打つと閉じている")
	}

	_, c = send(t, m, "esc")
	if c.Modal {
		t.Error("esc でモーダルが閉じない")
	}
}

// 孤児ユニットの行では詳細画面を開かない（対応するディレクトリが無い）。
func TestEnterOnOrphanDoesNotOpenModal(t *testing.T) {
	// G で一覧全体の末尾（孤児ユニットの区画）へ移る。
	m, _ := newModel(t, 80, 16)
	m, c := send(t, m, "G", "enter")
	if c.Modal {
		t.Error("孤児ユニットの行で詳細画面が開いている")
	}
	if !strings.Contains(m.View().Content, "old01.service") {
		t.Error("孤児ユニットの行が消えている")
	}
}

// / で絞り込みが始まり、入力中であることを親へ報告する。
func TestFilterReportsInput(t *testing.T) {
	m, _ := newModel(t, 80, 16)
	m, c := send(t, m, "/")
	if c.Input != "絞り込み" {
		t.Errorf("Input = %q, want 絞り込み", c.Input)
	}
	if !strings.Contains(c.Status, "入力中") {
		t.Errorf("状態行 = %q, 入力中を示していない", c.Status)
	}

	// 入力中の文字はそのまま入力欄へ入る（数字を含む runner 名で絞り込める）。
	m, c = send(t, m, "2")
	if c.Input == "" {
		t.Fatal("入力中に数字を打つと入力が終わっている")
	}
	if got := m.View().Content; !strings.Contains(got, "build01-2") || strings.Contains(got, "build01-1 ") {
		t.Errorf("絞り込みの結果が反映されていない: %q", got)
	}

	_, c = send(t, m, "esc")
	if c.Input != "" {
		t.Error("esc で入力が終わらない")
	}
}

// space で選択が増え、件数を状態行に報告する。
func TestSpaceSelects(t *testing.T) {
	m, c := newModel(t, 80, 16)
	if strings.Contains(c.Status, "選択") {
		t.Fatalf("初期状態の状態行 = %q, 選択件数が出ている", c.Status)
	}

	m, c = send(t, m, "space")
	if !strings.Contains(c.Status, "選択: 1 件") {
		t.Errorf("状態行 = %q, want 選択: 1 件", c.Status)
	}

	m, c = send(t, m, "j", "space")
	if !strings.Contains(c.Status, "選択: 2 件") {
		t.Errorf("状態行 = %q, want 選択: 2 件", c.Status)
	}

	// esc は選択のクリアを先に行う。
	_, c = send(t, m, "esc")
	if strings.Contains(c.Status, "選択") {
		t.Errorf("esc の後の状態行 = %q, 選択が残っている", c.Status)
	}
}

// 本体の領域が一覧へ伝わる。幅で列が落ち、高さで行数が収まる。
func TestBodySizeReachesTable(t *testing.T) {
	wide, _ := newModel(t, 80, 16)
	if !strings.Contains(wide.View().Content, token.ColVersion) {
		t.Errorf("幅 80 で VERSION 列が落ちている")
	}

	narrow, _ := newModel(t, 62, 8)
	got := narrow.View().Content
	if strings.Contains(got, token.ColVersion) {
		t.Errorf("幅 62 で VERSION 列が残っている: %q", got)
	}
	if n := len(strings.Split(got, "\n")); n > 8 {
		t.Errorf("高さ 8 に対して %d 行を描いている", n)
	}
}

// フッタは対象 runner の操作の可否と理由を持つ。
func TestFooterCarriesReasons(t *testing.T) {
	_, c := newModel(t, 80, 16)
	if len(c.Footer) == 0 {
		t.Fatal("フッタのヒントが空である")
	}
	for _, h := range c.Footer {
		if !h.Enabled && h.Reason == "" {
			t.Errorf("無効なキー %q に理由が無い", h.Key)
		}
	}

	// 孤児ユニットの区画では runner の操作を出さない。
	_, c = send(t, tea.Model(mustModel(t, 80, 16)), "G")
	for _, h := range c.Footer {
		if h.Key == "D" {
			t.Error("孤児ユニットの行で runner の削除キーを出している")
		}
	}
}

// mustModel は共有状態を配った Runners タブを返す。
func mustModel(t *testing.T, w, h int) tea.Model {
	t.Helper()

	m, _ := newModel(t, w, h)
	return m
}

// モーダル表示中のキーは背後の一覧へ届かない。
//
// overlay とモーダル同士の遮断は page 側で検証しているが、モーダル → 背後の
// organism.Table の経路は View がモーダルだけを返すため表示では気付けない。
// 選択件数（状態行）とカーソル位置で、一覧の状態が動いていないことを見る。
func TestModalKeysDoNotReachList(t *testing.T) {
	m, _ := newModel(t, 80, 16)
	before := cursorRow(t, m)

	m, c := send(t, m, "enter")
	if !c.Modal {
		t.Fatal("enter で詳細画面が開かない")
	}

	// space は一覧では選択、j はカーソル移動。モーダル表示中はどちらも背後へ届かない。
	m, c = send(t, m, "space", "j", "space")
	if strings.Contains(c.Status, "選択") {
		t.Errorf("モーダル表示中の状態行 = %q, 背後の一覧で選択が起きている", c.Status)
	}

	m, c = send(t, m, "esc")
	if c.Modal {
		t.Fatal("esc でモーダルが閉じない")
	}
	if strings.Contains(c.Status, "選択") {
		t.Errorf("閉じた後の状態行 = %q, 背後の一覧で選択が起きていた", c.Status)
	}
	if got := cursorRow(t, m); got != before {
		t.Errorf("閉じた後のカーソル行 = %q, want %q（背後の一覧でカーソルが動いた）", got, before)
	}
}

// cursorRow はカーソル記号が付いている行を返す。
func cursorRow(t *testing.T, m tea.Model) string {
	t.Helper()

	for _, line := range strings.Split(m.View().Content, "\n") {
		if strings.Contains(line, token.IconCursor) {
			return line
		}
	}
	t.Fatal("カーソル行が見つからない")
	return ""
}

// 移動キーの表記は詳細画面のフッタ（page.RunnerDetail.Hints）と揃える。
//
// keymap の Help().Key は "j/↓" のように別名を含む表記なので、そのまま使うと
// 同じフッタの中で移動キーの表記が 2 通り（"j/↓/k/↑" と "j/k"）になる。
func TestListHintsUseSameKeyNotationAsDetail(t *testing.T) {
	// 孤児ユニットの区画では runner の操作ではなく一覧の移動を出す。
	_, c := send(t, tea.Model(mustModel(t, 80, 16)), "G")
	if len(c.Footer) == 0 {
		t.Fatal("フッタのヒントが空である")
	}
	if got := c.Footer[0].Key; got != "j/k" {
		t.Errorf("移動キーの表記 = %q, want %q", got, "j/k")
	}
	if got := c.Footer[1].Key; got != "/" {
		t.Errorf("絞り込みのキーの表記 = %q, want %q", got, "/")
	}
}
