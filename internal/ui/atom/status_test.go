package atom

import (
	"testing"
	"time"

	"github.com/ousiassllc/gsr-helper/internal/ui/token"
)

// 記号と文字の両方を返す（色を使えない端末でも状態を判別できるようにするため）。
//
// 装飾を含まない素の文字列を返すため、期待値をそのまま書ける。列幅への
// 切り詰めは molecule が atom.Cell で行う。
func TestStatusText(t *testing.T) {
	cases := []struct {
		name     string
		active   string
		sub      string
		want     string
		wantRole token.RoleToken
	}{
		{"稼働中", "active", "running", token.IconActive + " active", token.RoleOK},
		{"停止中", "inactive", "dead", token.IconInactive + " inactive", token.RoleMuted},
		{"異常終了", "failed", "failed", token.IconFailed + " failed", token.RoleFail},
		{"ユニット無し", "", "", token.IconNoUnit, token.RoleMuted},
		{"起動中", "activating", "start-pre", token.IconWarn + " activating", token.RoleWarn},
		{"停止中への遷移", "deactivating", "stop-sigterm", token.IconWarn + " deactivating", token.RoleWarn},
		{"再読み込み中", "reloading", "reload", token.IconWarn + " reloading", token.RoleWarn},
		{"sub が failed", "active", "failed", token.IconFailed + " failed", token.RoleFail},
	}
	for _, c := range cases {
		got, role := StatusText(c.active, c.sub)
		if got != c.want {
			t.Errorf("%s: StatusText(%q, %q) = %q, want %q", c.name, c.active, c.sub, got, c.want)
		}
		if role != c.wantRole {
			t.Errorf("%s: StatusText(%q, %q) の役割 = %d, want %d", c.name, c.active, c.sub, role, c.wantRole)
		}
	}
}

func TestJobText(t *testing.T) {
	if got, role := JobText(false, 0); got != "idle" || role != token.RoleMuted {
		t.Errorf("JobText(false) = %q / %d, want %q / RoleMuted", got, role, "idle")
	}
	got, role := JobText(true, 4*time.Minute+12*time.Second)
	if want := token.IconJob + " 4m12s"; got != want || role != token.RoleAccent {
		t.Errorf("JobText(true) = %q / %d, want %q / RoleAccent", got, role, want)
	}
}

func TestCursor(t *testing.T) {
	s := plainStyles()
	if got, want := Cursor(true, s), token.IconCursor; got != want {
		t.Errorf("Cursor(true) = %q, want %q", got, want)
	}
	if got, want := Cursor(false, s), " "; got != want {
		t.Errorf("Cursor(false) = %q, want %q（記号と同じ幅の空白）", got, want)
	}
}

func TestCheckbox(t *testing.T) {
	s := plainStyles()
	cases := []struct {
		state CheckState
		want  string
	}{
		{CheckOn, token.IconChecked},
		{CheckOff, token.IconUnchecked},
		{CheckHidden, "   "},
	}
	for _, c := range cases {
		if got := Checkbox(c.state, s); got != c.want {
			t.Errorf("Checkbox(%d) = %q, want %q", c.state, got, c.want)
		}
	}
	// 3 状態の幅は揃っている（選択モードの切り替えで桁がずれない）。
	widths := map[int]bool{}
	for _, c := range cases {
		widths[len([]rune(Checkbox(c.state, s)))] = true
	}
	if len(widths) != 1 {
		t.Errorf("Checkbox の 3 状態で幅が揃っていない: %v", widths)
	}
}

func TestWarnMark(t *testing.T) {
	s := plainStyles()
	if got, want := WarnMark(true, s), token.IconWarn; got != want {
		t.Errorf("WarnMark(true) = %q, want %q", got, want)
	}
	if got, want := WarnMark(false, s), " "; got != want {
		t.Errorf("WarnMark(false) = %q, want %q", got, want)
	}
}
