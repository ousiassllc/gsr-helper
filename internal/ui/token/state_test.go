package token

import (
	"go/ast"
	"go/parser"
	gotoken "go/token"
	"strings"
	"testing"
)

// allStates は定義済みの状態トークンをすべて返す。
// 色と記号の対が揃っていることを走査するために使う。
func allStates() []StateToken {
	return []StateToken{StateOK, StateWarn, StateFail, StateSkip}
}

// allRoles は定義済みの役割トークンをすべて返す。
func allRoles() []RoleToken {
	return []RoleToken{
		RolePlain, RoleOK, RoleWarn, RoleFail, RoleSkip, RoleMuted, RoleAccent, RoleDanger,
	}
}

// 色だけで区別する状態を作らないため、状態には記号と色の両方がある。
//
// 対を要求するのは状態（OK / WARN / FAIL / SKIP）に限る。RoleMuted のような
// 表示上の役割は記号を持たないため、この検証の対象にしない。
func TestStateHasIconAndColor(t *testing.T) {
	for _, st := range allStates() {
		icon, role, ok := stateDisplay(st)
		if !ok {
			t.Errorf("状態 %d に表示の定義がない", st)
			continue
		}
		if icon == "" {
			t.Errorf("状態 %d に記号がない", st)
		}
		if _, ok := roleColor(role); !ok {
			t.Errorf("状態 %d の役割 %d に色の定義がない", st, role)
		}
		if Icon(st) != icon || st.Role() != role {
			t.Errorf("状態 %d の Icon / Role が stateDisplay と食い違う", st)
		}
	}

	// 定義の外側では未定義でなければならない（記号だけ増えることを防ぐ）。
	for _, st := range []StateToken{-1, StateToken(len(allStates()))} {
		if _, _, ok := stateDisplay(st); ok {
			t.Errorf("未定義の状態 %d に表示の定義がある", st)
		}
		if got := Icon(st); got != "" {
			t.Errorf("未定義の状態 %d の記号 = %q, want 空文字", st, got)
		}
		if got := st.Role(); got != RolePlain {
			t.Errorf("未定義の状態 %d の役割 = %d, want RolePlain", st, got)
		}
	}
}

// 装飾しない RolePlain 以外の役割は明暗の色を対で持つ。
func TestRoleColorHasLightAndDark(t *testing.T) {
	for _, role := range allRoles() {
		p, ok := roleColor(role)
		if role == RolePlain {
			if ok {
				t.Error("RolePlain に色の定義がある（装飾しない役割である）")
			}
			continue
		}
		if !ok {
			t.Errorf("役割 %d に色の定義がない", role)
			continue
		}
		if p.light == nil || p.dark == nil {
			t.Errorf("役割 %d の色が明暗の対になっていない", role)
		}
		if p.light == p.dark {
			t.Errorf("役割 %d の明背景用と暗背景用が同じ色である", role)
		}
	}

	// 定義の外側には色を持たせない。
	if _, ok := roleColor(RoleToken(len(allRoles()))); ok {
		t.Error("未定義の役割に色がある")
	}
}

// 記号は screens.md の記号表と一対一で対応する。
//
// **取りこぼしは icon.go を読んで検出する。** 手で並べた表だけでは、記号を足した
// ときにここへ足し忘れても緑のままになる。実際 IconUnknown（SVC 列の要）と
// IconEllipsis が抜けたまま「一対一」を主張していた（Issue #31）。
func TestIconsMatchSpec(t *testing.T) {
	cases := []struct {
		name string
		got  string
		want string
	}{
		{"IconActive", IconActive, "●"},
		{"IconInactive", IconInactive, "○"},
		{"IconFailed", IconFailed, "✗"},
		{"IconNoUnit", IconNoUnit, "-"},
		{"IconUnknown", IconUnknown, "?"},
		{"IconJob", IconJob, "▶"},
		{"IconWarn", IconWarn, "⚠"},
		{"IconOK", IconOK, "✓"},
		{"IconSkip", IconSkip, "⊘"},
		{"IconCursor", IconCursor, "▸"},
		{"IconChecked", IconChecked, "[x]"},
		{"IconUnchecked", IconUnchecked, "[ ]"},
		{"IconDivider", IconDivider, "─"},
		{"IconEllipsis", IconEllipsis, "…"},
	}

	covered := make(map[string]bool, len(cases))
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%s = %q, want %q", c.name, c.got, c.want)
		}
		covered[c.name] = true
	}

	for _, name := range iconConstNames(t) {
		if !covered[name] {
			t.Errorf("icon.go の %s がこの表に無い（記号を足したらここにも足すこと）", name)
		}
	}
}

// iconConstNames は icon.go が宣言する Icon* 定数の名前を返す。
//
// 定数は型の付かない文字列なので、実行時には列挙できない。宣言そのものを読むのが
// 「記号表と一対一」を機械的に確かめる唯一の方法である。
func iconConstNames(t *testing.T) []string {
	t.Helper()

	f, err := parser.ParseFile(gotoken.NewFileSet(), "icon.go", nil, 0)
	if err != nil {
		t.Fatalf("icon.go を読めない: %v", err)
	}

	var out []string
	for _, decl := range f.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != gotoken.CONST {
			continue
		}
		for _, spec := range gen.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for _, name := range vs.Names {
				if strings.HasPrefix(name.Name, "Icon") {
					out = append(out, name.Name)
				}
			}
		}
	}
	if len(out) == 0 {
		t.Fatal("icon.go から Icon* 定数を 1 つも読み出せなかった")
	}
	return out
}

// 状態の記号は Doctor の判定表示（✓ OK / ⚠ WARN / ✗ FAIL / ⊘ SKIP）に対応する。
func TestIcon(t *testing.T) {
	want := map[StateToken]string{
		StateOK:   IconOK,
		StateWarn: IconWarn,
		StateFail: IconFailed,
		StateSkip: IconSkip,
	}
	for st, w := range want {
		if got := Icon(st); got != w {
			t.Errorf("Icon(%d) = %q, want %q", st, got, w)
		}
	}
}
