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

// Normalize は設定を正規化した写しを返す（normalize の公開口）。
//
// **設定編集のフォームが差分を出す前に通すためにある。** Save は書き込む直前に
// normalize を通すので、正規化前の値で差分を組むと「承認した文字列」と「実際に
// 書かれる文字列」が食い違う（scan_roots の Clean、audit_log の既定値の補完、
// warn >= critical の拒否がここで起きる）。差分に出す内容と書き込む内容を同じ値
// から作る、という約束を自身の設定でも守るための口である。
func Normalize(c Config) (Config, error) { return normalize(c) }

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
	if err := ValidateRefresh("refresh_interval", c.RefreshInterval); err != nil {
		return Config{}, err
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

// normalizeRoots は走査ルートを検証する。空要素を捨て、残りを CleanScanRoot に通す。
//
// 相対パスは絶対化せず「絶対パスで指定してください」と拒否する（根拠は cleanAbs の
// doc）。重複除去はここでは行わず MergeScanRoots に集約してあり、設定ファイル内の
// 重複も --root と入口をまたいだ重複も同じ実装で落ちる。
func normalizeRoots(roots []string) ([]string, error) {
	out := make([]string, 0, len(roots))
	for _, r := range roots {
		if strings.TrimSpace(r) == "" {
			continue
		}
		p, err := CleanScanRoot("scan_roots", r)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return MergeScanRoots(out, nil), nil
}

// ValidateRefresh は自動更新間隔（秒）が有効範囲かを確かめる。
//
// **設定ファイルの refresh_interval と CLI の --refresh はこの 1 箇所を通す。**
// field は利用者に見せる項目名（"refresh_interval" / "--refresh"）。同じ設定に
// 入口ごとの有効範囲があると、--refresh 86400 は通るのに refresh_interval: 86400 は
// 起動を止めるという食い違いになる。
func ValidateRefresh(field string, sec int) error {
	if sec < minRefresh || sec > maxRefresh {
		return fmt.Errorf("%s は %d〜%d 秒で指定してください: %d", field, minRefresh, maxRefresh, sec)
	}
	return nil
}

// CleanScanRoot は走査ルートを検証して Clean した絶対パスを返す。
//
// **設定ファイルの scan_roots と CLI の --root はこの 1 箇所を通す。**
// field は利用者に見せる項目名（"scan_roots" / "--root"）。絶対パスと .. の検査は
// 安全上の根拠があり（cleanAbs の doc）、入口によって迂回できてはならない。
func CleanScanRoot(field, path string) (string, error) {
	return cleanAbs(field, strings.TrimSpace(path))
}

// MergeScanRoots は設定ファイルの scan_roots に --root の値を足す。
//
// 入口をまたいだ重複を除き、cfg → extra の順序を保つ。走査ルートの重複除去は
// ここだけにあり、設定ファイル内の重複も入口をまたいだ重複も同じ実装で落ちる。
// 双方の要素が CleanScanRoot を通った値であることを前提とする（同じルートが違う
// 表記のまま残ると重複と判定できない）。
func MergeScanRoots(cfg, extra []string) []string {
	out := make([]string, 0, len(cfg)+len(extra))
	for _, r := range slices.Concat(cfg, extra) {
		if r != "" && !slices.Contains(out, r) {
			out = append(out, r)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
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
