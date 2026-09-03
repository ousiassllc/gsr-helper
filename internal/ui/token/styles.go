package token

import "charm.land/lipgloss/v2"

// Styles は解決済みのスタイル一式。背景の明暗と色の有効無効を反映済みであり、
// 下位の階層はこの値を受け取るだけで環境を問い合わせない。
type Styles struct {
	OK, Warn, Fail, Skip, Muted, Accent, Danger lipgloss.Style
	Header                                      lipgloss.Style
	TabActive, TabInactive, TabDisabled         lipgloss.Style
	Cursor, Selected, Divider                   lipgloss.Style
}

// NewStyles は背景の明暗と色の有効無効からスタイルを解決する。
//
// color が false のときはすべて素通しのスタイルを返す。記号は色と対で
// 定義してあるため、素通しでも状態は判別できる。
//
// 仕様書（atomic-design.md）の token.Styles(dark) をそのままの名前にできないのは、
// 戻り値の型名 Styles と関数名が衝突するためである。引数に色の有効無効を足したのは、
// NO_COLOR / --no-color / 非 TTY の判定を cmd で 1 つの値に決めて渡す設計に合わせる。
func NewStyles(dark, color bool) Styles {
	plain := lipgloss.NewStyle()
	if !color {
		return Styles{
			OK: plain, Warn: plain, Fail: plain, Skip: plain,
			Muted: plain, Accent: plain, Danger: plain,
			Header:    plain,
			TabActive: plain, TabInactive: plain, TabDisabled: plain,
			Cursor: plain, Selected: plain, Divider: plain,
		}
	}

	fg := func(t RoleToken) lipgloss.Style {
		p, ok := roleColor(t)
		if !ok {
			return plain
		}
		if dark {
			return plain.Foreground(p.dark)
		}
		return plain.Foreground(p.light)
	}

	return Styles{
		OK:          fg(RoleOK),
		Warn:        fg(RoleWarn),
		Fail:        fg(RoleFail),
		Skip:        fg(RoleSkip),
		Muted:       fg(RoleMuted),
		Accent:      fg(RoleAccent),
		Danger:      fg(RoleDanger).Bold(true),
		Header:      fg(RoleMuted).Bold(true),
		TabActive:   fg(RoleAccent).Bold(true),
		TabInactive: fg(RoleMuted),
		TabDisabled: fg(RoleSkip),
		Cursor:      fg(RoleAccent).Bold(true),
		Selected:    fg(RoleAccent),
		Divider:     fg(RoleMuted),
	}
}

// Style は表示上の役割に対応するスタイルを返す。
// RolePlain と未定義の役割には素通しのスタイルを返す。
func (s Styles) Style(t RoleToken) lipgloss.Style {
	switch t {
	case RoleOK:
		return s.OK
	case RoleWarn:
		return s.Warn
	case RoleFail:
		return s.Fail
	case RoleSkip:
		return s.Skip
	case RoleMuted:
		return s.Muted
	case RoleAccent:
		return s.Accent
	case RoleDanger:
		return s.Danger
	case RolePlain:
		return lipgloss.NewStyle()
	default:
		return lipgloss.NewStyle()
	}
}
