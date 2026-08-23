package edit

import (
	"errors"
	"strconv"
	"strings"

	"github.com/ousiassllc/gsr-helper/internal/appconfig"
	"github.com/ousiassllc/gsr-helper/internal/config"
)

// 本ツール自身の設定（FR-41〜FR-42）の、tea にも huh にも依らない部分。
//
// 画面（page/config）に置かないのは、Config タブが 1 ディレクトリ 2000 行の上限に
// 対して画面だけで手一杯だからであり、また「フォームの文字列 → 設定 → 差分」の
// 変換は端末を起動せずに検証できるからである（Kind の doc と同じ理由）。
//
// **書き込みそのものは appconfig.Save に委ねる。** 一時ファイル + rename で常に
// 0600 かつ SUDO_USER の所有権にする処理を持っているのはそちらである。

// フォームの入力の形式に関するエラー。
var (
	// ErrBadNumber は数として読めない入力のエラー。
	ErrBadNumber = errors.New("数値を入力してください")
	// ErrPercentRange は割合が 1〜100 の外にある場合のエラー。
	ErrPercentRange = errors.New("1〜100 の範囲で入力してください")
)

// SelfValues は自身の設定フォームの入力先。
//
// すべて文字列なのは huh の入力欄がポインタで文字列を束縛するためである。
// 数値への変換と範囲の検証は Apply と Validate* が引き受ける。
type SelfValues struct {
	// ScanRoots は追加の走査ルート（カンマ区切りの絶対パス）。
	ScanRoots string
	// Refresh は一覧の自動更新間隔（秒）。
	Refresh string
	// Warn はディスク使用率の警告閾値（%）。
	Warn string
	// Critical はディスク使用率の危険閾値（%）。
	Critical string
	// AuditLog は監査ログの出力先（絶対パス）。
	AuditLog string
}

// NewSelfValues は現在の設定をフォームの初期値へ写す。
func NewSelfValues(c appconfig.Config) SelfValues {
	return SelfValues{
		ScanRoots: strings.Join(c.ScanRoots, ","),
		Refresh:   strconv.Itoa(c.RefreshInterval),
		Warn:      strconv.Itoa(c.DiskThresholds.Warn),
		Critical:  strconv.Itoa(c.DiskThresholds.Critical),
		AuditLog:  c.AuditLog,
	}
}

// Apply はフォームの入力を base へ反映し、正規化まで済ませた設定を返す。
//
// **正規化を差分より前に通すのが要点である。** appconfig.Save は書き込む直前に
// 同じ正規化を行うため（scan_roots の Clean、audit_log の既定値の補完）、
// 正規化前の値で差分を出すと承認した文字列と書かれる文字列が食い違う。
//
// 欄をまたぐ検証（warn < critical）もここで初めて効く。huh の Validate は 1 欄
// ずつしか見えないので、警告 > 危険という組み合わせはフォームでは弾けず、
// 正規化を通さないと Save まで気付けない。
func (v SelfValues) Apply(base appconfig.Config) (appconfig.Config, error) {
	out := base
	out.ScanRoots = SplitRoots(v.ScanRoots)

	var err error
	if out.RefreshInterval, err = atoiField(v.Refresh); err != nil {
		return base, err
	}
	if out.DiskThresholds.Warn, err = atoiField(v.Warn); err != nil {
		return base, err
	}
	if out.DiskThresholds.Critical, err = atoiField(v.Critical); err != nil {
		return base, err
	}
	out.AuditLog = strings.TrimSpace(v.AuditLog)

	next, err := appconfig.Normalize(out)
	if err != nil {
		return base, err
	}
	return next, nil
}

// RenderConfig は差分に出す設定の表現を返す。
//
// YAML そのものではなくキーと値の並びにするのは、編集した項目だけを差分に
// 出すためである。書き出す内容そのものは appconfig.Save が組み立てる。
func RenderConfig(c appconfig.Config) string {
	lines := []string{
		"scan_roots: " + strings.Join(c.ScanRoots, ","),
		"refresh_interval: " + strconv.Itoa(c.RefreshInterval),
		"disk_thresholds.warn: " + strconv.Itoa(c.DiskThresholds.Warn),
		"disk_thresholds.critical: " + strconv.Itoa(c.DiskThresholds.Critical),
		"audit_log: " + c.AuditLog,
	}
	return strings.Join(lines, "\n") + "\n"
}

// SplitRoots はカンマ区切りの走査ルートを分ける。
func SplitRoots(s string) []string {
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

// 以下は huh の Validate から 1 欄ずつ呼ぶ検証。規則は appconfig と
// internal/config のものをそのまま使う。設定ファイルから読む場合とフォームから
// 入れる場合で通る値が違うと、書いた設定で起動できなくなる。

// ValidateRoots は走査ルートの入力を検証する。
func ValidateRoots(s string) error {
	for _, r := range SplitRoots(s) {
		if _, err := appconfig.CleanScanRoot("scan_roots", r); err != nil {
			return err
		}
	}
	return nil
}

// ValidateRefresh は自動更新間隔の入力を検証する。
func ValidateRefresh(s string) error {
	n, err := atoiField(s)
	if err != nil {
		return err
	}
	return appconfig.ValidateRefresh("refresh_interval", n)
}

// ValidatePercent は割合が 1〜100 の整数かを見る。
//
// 警告 < 危険 の関係はここでは見られない（huh の Validate は 1 欄しか見えない）。
// 組み合わせの検証は Apply が正規化を通して行う。
func ValidatePercent(s string) error {
	n, err := atoiField(s)
	if err != nil {
		return err
	}
	if n < 1 || n > 100 {
		return ErrPercentRange
	}
	return nil
}

// ValidateAuditLog は監査ログの出力先を検証する。
func ValidateAuditLog(s string) error {
	_, err := config.ValidateAbsPath("監査ログ", strings.TrimSpace(s))
	return err
}

// atoiField は入力欄の文字列を整数にする。読めなければ ErrBadNumber を返す。
func atoiField(s string) (int, error) {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return 0, ErrBadNumber
	}
	return n, nil
}
