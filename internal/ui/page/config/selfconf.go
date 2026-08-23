package config

import (
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"

	"github.com/ousiassllc/gsr-helper/internal/appconfig"
	"github.com/ousiassllc/gsr-helper/internal/config"
	"github.com/ousiassllc/gsr-helper/internal/config/edit"
	"github.com/ousiassllc/gsr-helper/internal/ui/organism/dialog"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
)

// 自身の設定（FR-41〜FR-42）のフォーム。
//
// 初回起動時（設定ファイルが無い）は自動で開き、以後も対象の一覧から選んで
// 開き直せる。編集できるのは要件が挙げる 4 つ——走査ルート・ディスク閾値・
// ポーリング間隔・監査ログ出力先——である。
//
// **書き込みは appconfig.Save に委ねる。** 一時ファイル + rename で常に 0600 かつ
// SUDO_USER の所有権にする処理を持っているのはそちらであり、ここで書くと
// 所有権の扱いが 2 か所に分かれる。

// selfValues は自身の設定フォームの入力先。
type selfValues struct {
	scanRoots string
	refresh   string
	warn      string
	critical  string
	auditLog  string
}

// selfTitle はフォームの見出し。初回かどうかで変える。
const (
	selfTitleFirst = "初回設定（gsr-helper 自身の設定）"
	selfTitleEdit  = "gsr-helper 自身の設定"
)

// openSelfForm は自身の設定のフォームを開く（FR-41 / FR-42）。
func (m *Model) openSelfForm() tea.Cmd {
	m.self = true
	m.formShown = true
	m.vals.kind = edit.KindSelf
	m.vals.self = fromConfig(m.st.Config.Conf)

	title := selfTitleEdit
	if m.st.Config.FirstRun {
		title = selfTitleFirst
	}

	return m.overlay.Open(formKind, formOpenMsg{title: title, values: m.vals, st: m.st})
}

// fromConfig は現在の設定をフォームの初期値へ写す。
func fromConfig(c appconfig.Config) selfValues {
	return selfValues{
		scanRoots: strings.Join(c.ScanRoots, ","),
		refresh:   strconv.Itoa(c.RefreshInterval),
		warn:      strconv.Itoa(c.DiskThresholds.Warn),
		critical:  strconv.Itoa(c.DiskThresholds.Critical),
		auditLog:  c.AuditLog,
	}
}

// selfFields は自身の設定の入力欄を返す。
func (v *values) selfFields() []huh.Field {
	s := &v.self
	return []huh.Field{
		huh.NewInput().Title("追加の走査ルート").
			Description("カンマ区切りの絶対パス。空なら既定の場所だけを探します").
			Value(&s.scanRoots).Validate(validateRoots),
		huh.NewInput().Title("一覧の自動更新間隔（秒）").
			Description("1〜3600").Value(&s.refresh).Validate(validateRefresh),
		huh.NewInput().Title("ディスク使用率の警告閾値（%）").
			Description("1〜99").Value(&s.warn).Validate(validatePercent),
		huh.NewInput().Title("ディスク使用率の危険閾値（%）").
			Description("1〜100。警告より大きくします").Value(&s.critical).Validate(validatePercent),
		huh.NewInput().Title("監査ログの出力先").
			Description("絶対パス").Value(&s.auditLog).Validate(validateAbs),
	}
}

// apply はフォームの入力を設定へ反映した写しを返す。
func (v selfValues) apply(base appconfig.Config) (appconfig.Config, error) {
	out := base
	out.ScanRoots = splitRoots(v.scanRoots)

	var err error
	if out.RefreshInterval, err = strconv.Atoi(strings.TrimSpace(v.refresh)); err != nil {
		return base, errBadNumber
	}
	if out.DiskThresholds.Warn, err = strconv.Atoi(strings.TrimSpace(v.warn)); err != nil {
		return base, errBadNumber
	}
	if out.DiskThresholds.Critical, err = strconv.Atoi(strings.TrimSpace(v.critical)); err != nil {
		return base, errBadNumber
	}
	out.AuditLog = strings.TrimSpace(v.auditLog)

	return out, nil
}

// saveSelf は自身の設定を書き込む（FR-41 / FR-42）。
//
// 差分の承認はここでも経る。破壊的な書き込みであることは runner 側の設定と
// 変わらないためである（FR-37）。バックアップは appconfig.Save が一時ファイル +
// rename で置き換えるため、書き損じで元の設定が壊れることはない。
func (m *Model) saveSelf() tea.Cmd {
	next, err := m.vals.self.apply(m.st.Config.Conf)
	if err != nil {
		m.notice = err.Error()
		m.overlay.Close()

		return nil
	}

	before, after := renderConfig(m.st.Config.Conf), renderConfig(next)
	if before == after {
		m.notice = "変更はありません"
		m.overlay.Close()

		return nil
	}

	m.pendingSelf = next
	m.overlay.Close()

	return m.overlay.Open(diffKind, diffOpenMsg{input: dialog.DiffApprovalInput{
		Path:   m.st.Config.Path,
		Diff:   edit.SplitDiff(config.Diff(before, after)),
		Backup: "",
	}})
}

// commitSelf は承認された自身の設定を書き込む。
func (m *Model) commitSelf() tea.Cmd {
	cfg, path := m.pendingSelf, m.st.Config.Path
	m.busy = true

	return page.Do(m.tab, func() tea.Msg {
		if err := appconfig.Save(cfg, path); err != nil {
			return doneMsg{text: "", err: err}
		}
		return doneMsg{text: "設定を書き込みました", err: nil}
	})
}

// renderConfig は差分に出す設定の表現を返す。
//
// YAML そのものではなくキーと値の並びにするのは、編集した項目だけを差分に
// 出すためである。書き出す内容そのものは appconfig.Save が組み立てる。
func renderConfig(c appconfig.Config) string {
	lines := []string{
		"scan_roots: " + strings.Join(c.ScanRoots, ","),
		"refresh_interval: " + strconv.Itoa(c.RefreshInterval),
		"disk_thresholds.warn: " + strconv.Itoa(c.DiskThresholds.Warn),
		"disk_thresholds.critical: " + strconv.Itoa(c.DiskThresholds.Critical),
		"audit_log: " + c.AuditLog,
	}
	return strings.Join(lines, "\n") + "\n"
}

// splitRoots はカンマ区切りの走査ルートを分ける。
func splitRoots(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}

	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// 検証は appconfig の規則をそのまま使う。設定ファイルから読む場合と
// フォームから入れる場合で通る値が違うと、書いた設定で起動できなくなる。
func validateRoots(s string) error {
	for _, r := range splitRoots(s) {
		if _, err := appconfig.CleanScanRoot("scan_roots", r); err != nil {
			return err
		}
	}
	return nil
}

// validateRefresh は更新間隔を検証する。
func validateRefresh(s string) error {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return errBadNumber
	}
	return appconfig.ValidateRefresh("refresh_interval", n)
}

// validatePercent は 1〜100 の整数かを見る。範囲の詳細は appconfig が起動時に検証する。
func validatePercent(s string) error {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return errBadNumber
	}
	if n < 1 || n > 100 {
		return errPercentRange
	}
	return nil
}

// validateAbs は絶対パスかを見る。
func validateAbs(s string) error {
	_, err := config.ValidateWorkDir(strings.TrimSpace(s), 0, nil)

	return err
}
