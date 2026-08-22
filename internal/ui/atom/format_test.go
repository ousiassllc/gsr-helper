package atom

import (
	"testing"
	"time"

	"github.com/ousiassllc/gsr-helper/internal/ui/token"
)

func TestDuration(t *testing.T) {
	cases := []struct {
		d    time.Duration
		want string
	}{
		{0, "0s"},
		{59 * time.Second, "59s"},
		{time.Minute, "1m00s"},
		{4*time.Minute + 12*time.Second, "4m12s"},
		{22*time.Minute + 3*time.Second, "22m03s"},
		{time.Hour + 2*time.Minute, "1h02m"},
		{100*time.Hour + 30*time.Minute, "100h30m"},
		{-1 * time.Second, token.IconNoUnit},
		{-100 * time.Hour, token.IconNoUnit},
	}
	for _, c := range cases {
		if got := Duration(c.d); got != c.want {
			t.Errorf("Duration(%v) = %q, want %q", c.d, got, c.want)
		}
	}
}

func TestVersionText(t *testing.T) {
	cases := []struct {
		name     string
		cur      string
		latest   string
		want     string
		wantRole token.RoleToken
	}{
		{"同値なら注意記号なし", "2.311.0", "2.311.0", "2.311.0", token.RolePlain},
		{"差異があれば注意記号", "2.309.0", "2.311.0", "2.309.0 " + token.IconWarn, token.RoleWarn},
		{"最新が未取得なら注意記号なし", "2.309.0", "", "2.309.0", token.RolePlain},
		{"現行が空", "", "2.311.0", token.IconNoUnit, token.RoleMuted},
		{"長いバージョン", "2.311.0-beta.1", "2.311.0", "2.311.0-beta.1 " + token.IconWarn, token.RoleWarn},
	}
	for _, c := range cases {
		got, role := VersionText(c.cur, c.latest)
		if got != c.want {
			t.Errorf("%s: VersionText(%q, %q) = %q, want %q", c.name, c.cur, c.latest, got, c.want)
		}
		if role != c.wantRole {
			t.Errorf("%s: VersionText(%q, %q) の役割 = %d, want %d", c.name, c.cur, c.latest, role, c.wantRole)
		}
	}
}
