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

// systemd の管理状態が判定できない runner の行にも注意記号を出す。
//
// この状態の runner は「ユニットが無い」と区別できないまま run.sh 直起動として
// 表示されていた行であり、注意記号もそれで付いていた。管理状態が分からない方が
// 直起動と分かっているより要注意なので、記号を落としてはならない。
func TestUnavailableManagedRowIsWarned(t *testing.T) {
	st := testState(80, 16)
	r := sampleRunner("build01-9", false)
	r.UnitName, r.Svc, r.Managed = "", nil, runner.ManagedUnavailable
	st.Result = runner.Result{Runners: []runner.Runner{r}, OrphanUnits: nil, Warnings: nil}

	m, _ := runners.New(0, st).Update(st)
	for _, line := range strings.Split(m.View().Content, "\n") {
		if !strings.Contains(line, "build01-9") {
			continue
		}
		if !strings.Contains(line, token.IconWarn) {
			t.Errorf("管理状態が判定できない行 = %q, 注意記号が無い", line)
		}
		return
	}
	t.Fatal("対象の runner の行が見つからない")
}

// 状態を取得できなかったユニットは、ユニットが無い runner と SVC 列で書き分ける。
//
// systemctl show が失敗したユニットは値の無い状態として渡ってくる（internal/runner の
// プレースホルダ）。同じ "-" で描くと「サービス登録されていない」と誤読される。
func TestUnknownServiceStateIsDistinguished(t *testing.T) {
	st := testState(80, 16)
	r := sampleRunner("build01-9", false)
	// systemctl show が失敗したユニットのプレースホルダ（値が無い SvcState）。
	r.Svc = &runner.SvcState{Unit: r.UnitName}
	st.Result = runner.Result{Runners: []runner.Runner{r}, OrphanUnits: nil, Warnings: nil}

	m, _ := runners.New(0, st).Update(st)
	for _, line := range strings.Split(m.View().Content, "\n") {
		if !strings.Contains(line, "build01-9") {
			continue
		}
		if !strings.Contains(line, token.IconUnknown) {
			t.Errorf("状態が取れなかった行 = %q, want %q を含む", line, token.IconUnknown)
		}
		return
	}
	t.Fatal("対象の runner の行が見つからない")
}

// 背景色が濃色 → 淡色に切り替わると、一覧の中身も新しい配色で描き直される。
//
// 端末の背景色は tea.BackgroundColorMsg で後から届くため、App は token.Styles を
// 作り直して StateMsg で配り直す。一覧が受け取った配色を捨てると、周囲の枠だけが
// 淡色になり、中身は濃色向けの薄い色のまま白背景に残って読めなくなる
// （token/state.go の palette が 2 型を持つ理由そのもの）。
func TestBackgroundColorChangeRestylesList(t *testing.T) {
	darkState := testState(80, 16)
	darkState.Styles = token.NewStyles(true, true)
	darkState.Dark = true

	m, _ := runners.New(0, darkState).Update(darkState)
	// カーソル位置と選択を作ってから配色を切り替える（再スタイルで失われないこと）。
	m, _ = send(t, m, "j", " ")
	darkBody := m.View().Content

	darkCursor := token.NewStyles(true, true).Cursor.Render(token.IconCursor)
	if !strings.Contains(darkBody, darkCursor) {
		t.Fatalf("濃色のカーソルが出ていない（テストの前提が崩れている）:\n%q", darkBody)
	}

	lightState := testState(80, 16)
	lightState.Styles = token.NewStyles(false, true)
	lightState.Dark = false
	m, _ = m.Update(lightState)
	lightBody := m.View().Content

	lightCursor := token.NewStyles(false, true).Cursor.Render(token.IconCursor)
	if !strings.Contains(lightBody, lightCursor) {
		t.Errorf("背景色を切り替えても一覧が淡色の配色にならない:\n%q", lightBody)
	}
	if strings.Contains(lightBody, darkCursor) {
		t.Errorf("濃色向けの配色が一覧に残っている:\n%q", lightBody)
	}

	// スクロール位置・選択・絞り込みは再スタイルで失われない。
	if !strings.Contains(lightBody, "build01-2") || !strings.Contains(lightBody, "build01-1") {
		t.Errorf("再スタイルで行が失われた:\n%q", lightBody)
	}
}

// 自動更新でカーソルより上の runner が消えても、enter が開く詳細は同じ runner のまま。
//
// 3 秒ごとの再検出は runner が 1 台消えるだけで並びを詰める。カーソルを生の添字で
// 当て直すと選択が 1 つ下へずれ、利用者が選んだつもりの runner とは別の runner の
// 詳細が開く。サービス制御を実装した時点で「選んだつもりとは別の runner を停止する」
// に化けるため、一覧の側で識別子ごとに貼り直す。
func TestAutoRefreshKeepsSelectedRunner(t *testing.T) {
	st := testState(80, 16)
	st.Result.Runners = []runner.Runner{
		sampleRunner("build01-1", false),
		sampleRunner("build01-2", true),
		sampleRunner("build01-3", false),
	}
	m, _ := runners.New(0, st).Update(st)

	// カーソルを 2 台目へ置く。
	m, _ = send(t, m, "j")

	// 再検出で 1 台目が消える（並びが 1 つ詰まる）。
	next := st
	next.Result.Runners = []runner.Runner{
		sampleRunner("build01-2", true),
		sampleRunner("build01-3", false),
	}
	m, _ = m.Update(next)

	m, c := send(t, m, "enter")
	if !c.Modal {
		t.Fatal("enter で詳細画面が開かない")
	}
	body := m.View().Content
	if !strings.Contains(body, "build01-2") {
		t.Errorf("選んでいた runner とは別の詳細が開いた:\n%s", body)
	}
	if strings.Contains(body, "build01-3") {
		t.Errorf("1 つ下の runner の詳細が開いている:\n%s", body)
	}
}
