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
	l := pane.NewLog()
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
func TestLogKeepsTailOnResize(t *testing.T) {
	l := newLogPane(20)

	l.SetSize(40, 8)
	if got := l.View(); !strings.Contains(got, "line19") {
		t.Errorf("リサイズ後に末尾が見えていない:\n%s", got)
	}
}

// 渡したスライスを書き換えても表示は崩れない（写しを取って渡している）。
func TestLogCopiesContent(t *testing.T) {
	l := pane.NewLog()
	l.SetSize(40, 5)
	lines := logLines(3)
	l.SetContent(lines)

	lines[0] = "書き換えた"
	if got := l.View(); strings.Contains(got, "書き換えた") {
		t.Errorf("呼び出し側のスライスの書き換えが表示に出た:\n%s", got)
	}
}
