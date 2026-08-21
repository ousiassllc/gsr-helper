package appconfig

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"unicode"
)

// 範囲の上下限。設定ファイルは手編集される前提なので、桁の打ち間違いを起動時に落とす。
const (
	minScanDepth    = 1
	maxScanDepth    = 10
	minRefresh      = 1
	maxRefresh      = 3600
	minThreshold    = 1
	maxDiskWarn     = 99
	maxDiskCritical = 100
)

// normalize は設定を正規化する純粋関数。Load と Save の両方が通す。
//
// 0 や空文字は「未指定」として既定値で埋め、構造的に誤った値（範囲外・相対パス・
// warn >= critical など）はエラーにする。ゼロ値を未指定と読み替えるのは、
// yaml.v3 が「キーが無い」と「0 と書いた」を区別しないためである。
func normalize(c Config) (Config, error) {
	roots, err := normalizeRoots(c.ScanRoots)
	if err != nil {
		return Config{}, err
	}
	c.ScanRoots = roots

	if c.ScanDepth == 0 {
		c.ScanDepth = defaultScanDepth
	}
	if c.ScanDepth < minScanDepth || c.ScanDepth > maxScanDepth {
		return Config{}, fmt.Errorf("scan_depth は %d〜%d で指定してください: %d", minScanDepth, maxScanDepth, c.ScanDepth)
	}

	if c.RefreshInterval == 0 {
		c.RefreshInterval = defaultRefreshInterval
	}
	if c.RefreshInterval < minRefresh || c.RefreshInterval > maxRefresh {
		return Config{}, fmt.Errorf("refresh_interval は %d〜%d 秒で指定してください: %d", minRefresh, maxRefresh, c.RefreshInterval)
	}

	if c.DiskThresholds, err = normalizeThresholds(c.DiskThresholds); err != nil {
		return Config{}, err
	}
	if c.AuditLog, err = normalizeAbsPath("audit_log", c.AuditLog, defaultAuditLog); err != nil {
		return Config{}, err
	}
	if c.Defaults, err = normalizeDefaults(c.Defaults); err != nil {
		return Config{}, err
	}
	return c, nil
}

// normalizeDefaults は runner 追加時の既定値を正規化する。
func normalizeDefaults(d Defaults) (Defaults, error) {
	var err error
	if d.NamePrefix, err = normalizeName("defaults.name_prefix", d.NamePrefix); err != nil {
		return Defaults{}, err
	}
	if d.InstallBase, err = normalizeAbsPath("defaults.install_base", d.InstallBase, defaultInstallBase); err != nil {
		return Defaults{}, err
	}
	if d.Labels, err = normalizeLabels(d.Labels); err != nil {
		return Defaults{}, err
	}
	return d, nil
}

// normalizeRoots は走査ルートを絶対パスに揃え、空要素と重複を落とす。
func normalizeRoots(roots []string) ([]string, error) {
	out := make([]string, 0, len(roots))
	for _, r := range roots {
		r = strings.TrimSpace(r)
		if r == "" {
			continue
		}
		p, err := cleanAbs("scan_roots", r)
		if err != nil {
			return nil, err
		}
		if !slices.Contains(out, p) {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

// normalizeThresholds は使用率の閾値を埋め、大小関係まで含めて検証する。
func normalizeThresholds(t DiskThresholds) (DiskThresholds, error) {
	if t.Warn == 0 {
		t.Warn = defaultDiskWarn
	}
	if t.Critical == 0 {
		t.Critical = defaultDiskCritical
	}
	switch {
	case t.Warn < minThreshold || t.Warn > maxDiskWarn:
		return DiskThresholds{}, fmt.Errorf("disk_thresholds.warn は %d〜%d で指定してください: %d", minThreshold, maxDiskWarn, t.Warn)
	case t.Critical < minThreshold || t.Critical > maxDiskCritical:
		return DiskThresholds{}, fmt.Errorf("disk_thresholds.critical は %d〜%d で指定してください: %d", minThreshold, maxDiskCritical, t.Critical)
	case t.Warn >= t.Critical:
		return DiskThresholds{}, fmt.Errorf("disk_thresholds.warn は critical より小さい値にしてください: warn=%d critical=%d", t.Warn, t.Critical)
	default:
		return t, nil
	}
}

// normalizeAbsPath は空なら def で埋め、絶対パスであることを確かめる。
func normalizeAbsPath(field, p, def string) (string, error) {
	if strings.TrimSpace(p) == "" {
		return def, nil
	}
	return cleanAbs(field, p)
}

// cleanAbs は絶対パスであることと .. を含まないことを確かめて Clean した値を返す。
//
// .. は Clean で消えてしまうため Clean 前の値で判定する。設定に書かれたパスから
// 実際の対象が読み取れない状態を許さない（走査ルートと監査ログの出力先は
// 削除やログ出力の対象になるため、意図の取り違えを起こしたくない）。
func cleanAbs(field, p string) (string, error) {
	switch {
	case slices.Contains(strings.Split(p, string(filepath.Separator)), ".."):
		return "", fmt.Errorf("%s に .. は使えません: %s", field, p)
	case !filepath.IsAbs(p):
		return "", fmt.Errorf("%s は絶対パスで指定してください: %s", field, p)
	default:
		return filepath.Clean(p), nil
	}
}

// normalizeName は runner 名のプレフィクスを検証する。空は許す。
//
// 厳密な文字種検証は internal/config.Validate* の担当なので、ここでは
// 「- で始まらない」「空白を含まない」の最小限に留める。二重に実装すると
// 規則が食い違い、どちらが正しいのか分からなくなるためである。
func normalizeName(field, s string) (string, error) {
	s = strings.TrimSpace(s)
	switch {
	case s == "":
		return "", nil
	case strings.HasPrefix(s, "-"):
		return "", fmt.Errorf("%s は - で始められません: %s", field, s)
	case strings.ContainsFunc(s, unicode.IsSpace):
		return "", fmt.Errorf("%s に空白文字は使えません: %s", field, s)
	default:
		return s, nil
	}
}

// normalizeLabels はラベルを trim し、空要素と重複を落とす。
// 文字種の検証範囲は normalizeName と同じ理由で最小限に留める。
func normalizeLabels(labels []string) ([]string, error) {
	out := make([]string, 0, len(labels))
	for _, l := range labels {
		l = strings.TrimSpace(l)
		if l == "" {
			continue
		}
		if strings.HasPrefix(l, "-") {
			return nil, fmt.Errorf("defaults.labels に - で始まる値は使えません: %s", l)
		}
		if !slices.Contains(out, l) {
			out = append(out, l)
		}
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}
