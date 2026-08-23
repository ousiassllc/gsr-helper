package pane_test

import (
	"strconv"
	"strings"
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/ui/organism/pane"
)

// logLines は n 行のログを返す。行番号を本文にして位置を判定できるようにする。
func logLines(n int) []string {
	out := make([]string, 0, n)
	for i := range n {
		out = append(out, "line"+strconv.Itoa(i))
	}
	return out
}

// newLogPane は大きさを決めたログ本文の領域を返す。
func newLogPane(lines int) pane.Log {
	l := pane.NewLog(testStyles())
	l.SetSize(40, 5)
	l.SetContent(logLines(lines))
	return l
}

// 追従は ON で始まり、内容を差し替えても末尾を表示し続ける（screens.md の Logs タブ）。
func TestLogFollowsTailOnNewContent(t *testing.T) {
	l := newLogPane(20)
	if !l.Following() {
		t.Fatal("初期状態で追従していない")
	}
	if got := l.View(); !strings.Contains(got, "line19") {
		t.Errorf("末尾が見えていない:\n%s", got)
	}

	l.SetContent(append(logLines(20), "line20"))
	if got := l.View(); !strings.Contains(got, "line20") {
		t.Errorf("追記後も末尾へ寄っていない:\n%s", got)
	}
}

// 手動で上へスクロールすると追従が切れ、以後の追記で末尾へ飛ばない。
func TestLogManualScrollStopsFollowing(t *testing.T) {
	l := newLogPane(20)

	l, _ = l.Update(press("k"))
	if l.Following() {
		t.Fatal("上へスクロールしても追従が続いている")
	}

	before := l.View()
	l.SetContent(append(logLines(20), "line20"))
	if got := l.View(); got != before {
		t.Errorf("追従を切ったのに表示が動いた:\nbefore:\n%s\nafter:\n%s", before, got)
	}
}

// 末尾に居るまま下へ打っても追従は切れない（位置が変わらないので切る理由が無い）。
func TestLogStaysFollowingAtBottom(t *testing.T) {
	l := newLogPane(20)

	l, _ = l.Update(press("j"))
	if !l.Following() {
		t.Error("末尾で j を押しただけで追従が切れた")
	}
}

// SetFollow(true) は末尾へ移す（`G` で追従を再開する経路）。
func TestLogSetFollowJumpsToBottom(t *testing.T) {
	l := newLogPane(20)
	l, _ = l.Update(press("k"))
	if l.Following() {
		t.Fatal("前提が崩れている（追従が切れていない）")
	}

	l.SetFollow(true)
	if !l.Following() {
		t.Fatal("追従を再開できていない")
	}
	if got := l.View(); !strings.Contains(got, "line19") {
		t.Errorf("再開しても末尾へ移っていない:\n%s", got)
	}
}

// SetFollow(false) は位置を動かさない（`f` で追従だけを止める）。
func TestLogSetFollowOffKeepsPosition(t *testing.T) {
	l := newLogPane(20)
	before := l.View()

	l.SetFollow(false)
	if l.Following() {
		t.Fatal("追従が切れていない")
	}
	if got := l.View(); got != before {
		t.Errorf("追従を止めただけで表示が動いた:\nbefore:\n%s\nafter:\n%s", before, got)
	}
}

// 大きさを変えても追従中なら末尾に居続ける（高さが変われば末尾の位置も変わる）。
//
// **縮める向きまで見る。** viewport は高さを変えても表示の先頭（yOffset）を動かさない
// ので、広げる操作では末尾が画面に残ったまま余白が増えるだけであり、SetSize の寄せ直し
// （follow なら GotoBottom）を消しても末尾が見えてしまう。それだけを見る検査は空振り
// する。広げてから縮めると、先頭が据え置かれた分だけ末尾が画面の外へ出るため、
// 寄せ直しの有無がそのまま表示に現れる。
func TestLogKeepsTailOnResize(t *testing.T) {
	l := newLogPane(20)

	l.SetSize(40, 10)
	if got := l.View(); !strings.Contains(got, "line19") {
		t.Errorf("高さを広げた後に末尾が見えていない:\n%s", got)
	}

	l.SetSize(40, 3)
	if got := l.View(); !strings.Contains(got, "line19") {
		t.Errorf("高さを縮めた後に末尾が見えていない:\n%s", got)
	}
}

// 渡したスライスを書き換えても表示は崩れない（写しを取って渡している）。
func TestLogCopiesContent(t *testing.T) {
	l := pane.NewLog(testStyles())
	l.SetSize(40, 5)
	lines := logLines(3)
	l.SetContent(lines)

	lines[0] = "書き換えた"
	if got := l.View(); strings.Contains(got, "書き換えた") {
		t.Errorf("呼び出し側のスライスの書き換えが表示に出た:\n%s", got)
	}
}

// フィルタは入力中だけカーソル付きの入力欄を、確定後は確定値を見出しへ返す。
func TestLogFilterLifecycle(t *testing.T) {
	l := newLogPane(5)
	if l.FilterView() != "" {
		t.Errorf("初期のフィルタ表記 = %q, want 空", l.FilterView())
	}

	l.StartFilter()
	if !l.Filtering() {
		t.Fatal("入力モードに入っていない")
	}
	l, _ = l.Update(press("E"))
	l, _ = l.Update(press("R"))
	l.AcceptFilter()

	if l.Filtering() {
		t.Error("確定しても入力モードのままである")
	}
	if got := l.Filter(); got != "ER" {
		t.Errorf("確定値 = %q, want ER", got)
	}
	if got := l.FilterView(); !strings.Contains(got, "ER") {
		t.Errorf("見出しのフィルタ表記 = %q, want ER を含む", got)
	}
}

// 取消は確定済みの値へ戻す（打ちかけの文字を残さない）。
func TestLogFilterCancelRestoresApplied(t *testing.T) {
	l := newLogPane(5)
	l.StartFilter()
	l, _ = l.Update(press("A"))
	l.AcceptFilter()

	l.StartFilter()
	l, _ = l.Update(press("B"))
	l.CancelFilter()

	if got := l.Filter(); got != "A" {
		t.Errorf("取消後の確定値 = %q, want A", got)
	}
	l.StartFilter()
	if got := l.FilterView(); strings.Contains(got, "AB") {
		t.Errorf("取消したはずの入力が残っている: %q", got)
	}
}

// 解除するとフィルタが空になる（入力中でないときの esc）。
func TestLogClearFilter(t *testing.T) {
	l := newLogPane(5)
	l.StartFilter()
	l, _ = l.Update(press("A"))
	l.AcceptFilter()

	l.ClearFilter()
	if got := l.Filter(); got != "" {
		t.Errorf("解除後の確定値 = %q, want 空", got)
	}
	if got := l.FilterView(); got != "" {
		t.Errorf("解除後の表記 = %q, want 空", got)
	}
}

// 入力中の打鍵はスクロールへ流さない（j / k / G を入力欄へ入れる）。
func TestLogFilterSwallowsScrollKeys(t *testing.T) {
	l := newLogPane(20)
	l.StartFilter()

	before := l.View()
	l, _ = l.Update(press("k"))
	if got := l.View(); got != before {
		t.Errorf("入力中の k でスクロールした:\nbefore:\n%s\nafter:\n%s", before, got)
	}
	l.AcceptFilter()
	if got := l.Filter(); got != "k" {
		t.Errorf("入力中の打鍵 = %q, want k（入力欄へ入る）", got)
	}
}
